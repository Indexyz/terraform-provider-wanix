// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kubeschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
)

var _ resource.Resource = &InstanceResource{}
var _ resource.ResourceWithConfigure = &InstanceResource{}
var _ resource.ResourceWithImportState = &InstanceResource{}

var instanceGVR = kubeschema.GroupVersionResource{
	Group:    "pop.wanix.net",
	Version:  "v1alpha1",
	Resource: "instances",
}

const (
	instanceAPIVersion = "pop.wanix.net/v1alpha1"
	instanceKind       = "Instance"
	defaultNamespace   = "default"
	defaultWaitSeconds = 600
)

var crdFieldNames = map[string]string{
	"disable_workload":         "disableWorkload",
	"workload_kind":            "workloadKind",
	"use_gateway":              "useGateway",
	"virtual_device":           "virtualDevice",
	"private_key":              "privateKey",
	"public_key":               "publicKey",
	"allowed_ips":              "allowedIPs",
	"persistent_keepalive":     "persistentKeepalive",
	"masquerade_interfaces":    "masqueradeInterfaces",
	"port_forwarding":          "portForwarding",
	"image_pull_secrets":       "imagePullSecrets",
	"network_setup":            "networkSetup",
	"pod_template_labels":      "podTemplateLabels",
	"pod_template_annotations": "podTemplateAnnotations",
}

func crdFieldName(name string) string {
	if crdName, ok := crdFieldNames[name]; ok {
		return crdName
	}

	return name
}

// InstanceResource manages pop.wanix.net/v1alpha1 Instance objects without
// using Kubernetes merge or server-side apply patches. Updates use the
// Kubernetes Update API with the current resourceVersion instead.
type InstanceResource struct {
	client dynamic.Interface
}

// InstanceResourceModel describes the Terraform representation of an Instance.
type InstanceResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Namespace          types.String `tfsdk:"namespace"`
	Labels             types.Map    `tfsdk:"labels"`
	Annotations        types.Map    `tfsdk:"annotations"`
	Spec               types.Object `tfsdk:"spec"`
	WaitForReady       types.Bool   `tfsdk:"wait_for_ready"`
	WaitTimeoutSeconds types.Int64  `tfsdk:"wait_timeout_seconds"`
	Phase              types.String `tfsdk:"phase"`
	Message            types.String `tfsdk:"message"`
	PodIPs             types.List   `tfsdk:"pod_ips"`
}

func NewInstanceResource() resource.Resource {
	return &InstanceResource{}
}

func (r *InstanceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_instance"
}

func (r *InstanceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = instanceResourceSchema()
}

func (r *InstanceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*kubernetesProviderConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Configuration",
			fmt.Sprintf("Expected *kubernetesProviderConfig, got %T.", req.ProviderData),
		)
		return
	}

	client, err := config.dynamicClient()
	if err != nil {
		resp.Diagnostics.AddError("Unable to Configure Kubernetes Client", err.Error())
		return
	}

	r.client = client
}

func (r *InstanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data InstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, ok := r.resourceClient(&resp.Diagnostics)
	if !ok {
		return
	}

	object, err := instanceObjectFromPlan(&data)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Build Instance", err.Error())
		return
	}

	created, err := client.Namespace(data.Namespace.ValueString()).Create(ctx, object, metav1.CreateOptions{})
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Instance", err.Error())
		return
	}

	data.ID = types.StringValue(instanceID(data.Namespace.ValueString(), data.Name.ValueString()))
	if err := setInstanceState(&data, created); err != nil {
		resp.Diagnostics.AddError("Unable to Decode Created Instance", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	created, err = r.waitForInstance(ctx, &data, created)
	if err != nil {
		resp.Diagnostics.AddError("Instance Did Not Become Ready", err.Error())
		return
	}

	if err := setInstanceState(&data, created); err != nil {
		resp.Diagnostics.AddError("Unable to Read Created Instance", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InstanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data InstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, ok := r.resourceClient(&resp.Diagnostics)
	if !ok {
		return
	}

	namespace, name, err := instanceAddress(&data)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Instance ID", err.Error())
		return
	}

	object, err := client.Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Instance", err.Error())
		return
	}

	data.ID = types.StringValue(instanceID(namespace, name))
	if err := setInstanceState(&data, object); err != nil {
		resp.Diagnostics.AddError("Unable to Decode Instance", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InstanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, prior InstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, ok := r.resourceClient(&resp.Diagnostics)
	if !ok {
		return
	}

	namespace, name := data.Namespace.ValueString(), data.Name.ValueString()
	var updated *unstructured.Unstructured
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := client.Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		desired, err := instanceObjectForUpdate(&data, &prior, current)
		if err != nil {
			return err
		}

		updated, err = client.Namespace(namespace).Update(ctx, desired, metav1.UpdateOptions{})
		return err
	})
	if apierrors.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Update Instance", err.Error())
		return
	}

	data.ID = types.StringValue(instanceID(namespace, name))
	if err := setInstanceState(&data, updated); err != nil {
		resp.Diagnostics.AddError("Unable to Decode Updated Instance", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	updated, err = r.waitForInstance(ctx, &data, updated)
	if err != nil {
		resp.Diagnostics.AddError("Instance Did Not Become Ready", err.Error())
		return
	}

	if err := setInstanceState(&data, updated); err != nil {
		resp.Diagnostics.AddError("Unable to Read Updated Instance", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InstanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data InstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, ok := r.resourceClient(&resp.Diagnostics)
	if !ok {
		return
	}

	namespace, name, err := instanceAddress(&data)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Instance ID", err.Error())
		return
	}

	err = client.Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to Delete Instance", err.Error())
		return
	}

	if err := r.waitForDeletion(ctx, namespace, name, waitDuration(data)); err != nil {
		resp.Diagnostics.AddError("Instance Was Not Deleted", err.Error())
	}
}

func (r *InstanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid Instance Import ID",
			"Import IDs must use the form <namespace>/<name>.",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("namespace"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("wait_for_ready"), true)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("wait_timeout_seconds"), int64(defaultWaitSeconds))...)
}

func (r *InstanceResource) resourceClient(diags *diag.Diagnostics) (dynamic.NamespaceableResourceInterface, bool) {
	if r.client == nil {
		diags.AddError("Kubernetes Client Is Not Configured", "Configure the provider before using wanix_instance.")
		return nil, false
	}

	return r.client.Resource(instanceGVR), true
}

func instanceID(namespace, name string) string {
	return namespace + "/" + name
}

func instanceAddress(data *InstanceResourceModel) (string, string, error) {
	namespace := data.Namespace.ValueString()
	name := data.Name.ValueString()
	if namespace != "" && name != "" {
		return namespace, name, nil
	}

	parts := strings.Split(data.ID.ValueString(), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected namespace/name, got %q", data.ID.ValueString())
	}

	return parts[0], parts[1], nil
}

func instanceObjectFromPlan(data *InstanceResourceModel) (*unstructured.Unstructured, error) {
	spec, err := attrValueToUnstructured(data.Spec)
	if err != nil {
		return nil, fmt.Errorf("convert spec: %w", err)
	}

	specMap, ok := spec.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("spec must be an object")
	}

	metadata := map[string]interface{}{
		"name":      data.Name.ValueString(),
		"namespace": data.Namespace.ValueString(),
	}

	if !data.Labels.IsNull() {
		labels, err := attrValueToUnstructured(data.Labels)
		if err != nil {
			return nil, fmt.Errorf("convert labels: %w", err)
		}
		metadata["labels"] = labels
	}
	if !data.Annotations.IsNull() {
		annotations, err := attrValueToUnstructured(data.Annotations)
		if err != nil {
			return nil, fmt.Errorf("convert annotations: %w", err)
		}
		metadata["annotations"] = annotations
	}

	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": instanceAPIVersion,
		"kind":       instanceKind,
		"metadata":   metadata,
		"spec":       specMap,
	}}, nil
}

func instanceObjectForUpdate(data, prior *InstanceResourceModel, current *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	desired := current.DeepCopy()
	spec, err := attrValueToUnstructured(data.Spec)
	if err != nil {
		return nil, fmt.Errorf("convert spec: %w", err)
	}
	desired.Object["spec"] = spec

	metadata, ok := desired.Object["metadata"].(map[string]interface{})
	if !ok {
		metadata = map[string]interface{}{}
		desired.Object["metadata"] = metadata
	}
	if err := updateManagedMetadataMap(metadata, "labels", data.Labels, prior.Labels); err != nil {
		return nil, err
	}
	if err := updateManagedMetadataMap(metadata, "annotations", data.Annotations, prior.Annotations); err != nil {
		return nil, err
	}

	return desired, nil
}

func updateManagedMetadataMap(metadata map[string]interface{}, key string, desired, prior types.Map) error {
	if desired.IsUnknown() {
		return fmt.Errorf("metadata.%s is unknown", key)
	}

	current, _ := metadata[key].(map[string]interface{})
	if current == nil {
		current = map[string]interface{}{}
	} else {
		cloned := make(map[string]interface{}, len(current))
		for name, value := range current {
			cloned[name] = value
		}
		current = cloned
	}

	for name := range prior.Elements() {
		delete(current, name)
	}

	elements, err := stringMapValue(desired)
	if err != nil {
		return err
	}
	for name := range elements {
		delete(current, name)
	}
	for name, value := range elements {
		current[name] = value
	}

	if len(current) == 0 {
		delete(metadata, key)
	} else {
		metadata[key] = current
	}

	return nil
}

func stringMapValue(value types.Map) (map[string]string, error) {
	if value.IsNull() {
		return nil, nil
	}
	if value.IsUnknown() {
		return nil, fmt.Errorf("map value is unknown")
	}

	result := make(map[string]string, len(value.Elements()))
	for key, element := range value.Elements() {
		stringValue, ok := element.(types.String)
		if !ok || stringValue.IsNull() || stringValue.IsUnknown() {
			return nil, fmt.Errorf("map value %q must be a known string", key)
		}
		result[key] = stringValue.ValueString()
	}

	return result, nil
}

func setInstanceState(data *InstanceResourceModel, object *unstructured.Unstructured) error {
	data.Name = types.StringValue(object.GetName())
	data.Namespace = types.StringValue(object.GetNamespace())
	data.ID = types.StringValue(instanceID(object.GetNamespace(), object.GetName()))

	if data.Spec.IsNull() {
		data.Labels = metadataMapValue(object.GetLabels())
		data.Annotations = metadataMapValue(object.GetAnnotations())
	}
	specRaw, found, err := unstructured.NestedFieldCopy(object.Object, "spec")
	if err != nil {
		return fmt.Errorf("read spec: %w", err)
	}
	if !found {
		data.Spec = types.ObjectNull(instanceSpecType().AttributeTypes())
	} else {
		if fields, ok := specRaw.(map[string]interface{}); ok {
			switch cpu := fields["cpu"].(type) {
			case int64, float64:
				fields["cpu"] = fmt.Sprint(cpu)
			}
		}
		specRaw = restoreOmittedZeroValues(specRaw, data.Spec)
		value, err := attrValueFromUnstructured(specRaw, instanceSpecType())
		if err != nil {
			return fmt.Errorf("decode spec: %w", err)
		}
		spec, ok := value.(types.Object)
		if !ok {
			return fmt.Errorf("decoded spec must be an object, got %T", value)
		}
		data.Spec = spec
	}

	labels := metadataMapValue(managedMetadata(object.GetLabels(), data.Labels))
	if labels.IsNull() && !data.Labels.IsNull() {
		labels = types.MapValueMust(types.StringType, map[string]attr.Value{})
	}
	data.Labels = labels

	annotations := metadataMapValue(managedMetadata(object.GetAnnotations(), data.Annotations))
	if annotations.IsNull() && !data.Annotations.IsNull() {
		annotations = types.MapValueMust(types.StringType, map[string]attr.Value{})
	}
	data.Annotations = annotations

	status, _ := object.Object["status"].(map[string]interface{})
	if status == nil {
		data.Phase = types.StringNull()
		data.Message = types.StringNull()
		data.PodIPs = types.ListNull(types.StringType)
		return nil
	}

	phase, _ := status["phase"].(string)
	if phase == "" {
		data.Phase = types.StringNull()
	} else {
		data.Phase = types.StringValue(phase)
	}
	message, _ := status["message"].(string)
	if message == "" {
		data.Message = types.StringNull()
	} else {
		data.Message = types.StringValue(message)
	}

	podIPs, _ := status["podIPs"].([]interface{})
	ipValues := make([]attr.Value, 0, len(podIPs))
	for _, podIP := range podIPs {
		if ip, ok := podIP.(string); ok {
			ipValues = append(ipValues, types.StringValue(ip))
		}
	}
	data.PodIPs = types.ListValueMust(types.StringType, ipValues)

	return nil
}

func metadataMapValue(values map[string]string) types.Map {
	if len(values) == 0 {
		return types.MapNull(types.StringType)
	}

	elements := make(map[string]attr.Value, len(values))
	for key, value := range values {
		elements[key] = types.StringValue(value)
	}

	return types.MapValueMust(types.StringType, elements)
}

func (r *InstanceResource) waitForInstance(ctx context.Context, data *InstanceResourceModel, initial *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	if data.WaitForReady.IsNull() || data.WaitForReady.IsUnknown() || !data.WaitForReady.ValueBool() {
		return initial, nil
	}

	timeout := waitDuration(*data)
	if timeout <= 0 {
		return nil, fmt.Errorf("waitTimeoutSeconds must be greater than zero")
	}

	var latest *unstructured.Unstructured
	err := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		object, err := r.client.Resource(instanceGVR).Namespace(data.Namespace.ValueString()).Get(ctx, data.Name.ValueString(), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		latest = object
		phase := nestedString(object.Object, "status", "phase")
		if phase == "Error" {
			message := nestedString(object.Object, "status", "message")
			if message == "" {
				message = "the operator reported phase Error"
			}
			return false, fmt.Errorf("%s", message)
		}

		paused, _, _ := unstructured.NestedBool(object.Object, "spec", "pause")
		ips, _, _ := unstructured.NestedStringSlice(object.Object, "status", "podIPs")
		return phase == "Ready" && (paused || len(ips) > 0), nil
	})
	if err != nil {
		return nil, err
	}

	return latest, nil
}

func (r *InstanceResource) waitForDeletion(ctx context.Context, namespace, name string, timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("waitTimeoutSeconds must be greater than zero")
	}

	return wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		_, err := r.client.Resource(instanceGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return false, nil
	})
}

func waitDuration(data InstanceResourceModel) time.Duration {
	if data.WaitTimeoutSeconds.IsNull() || data.WaitTimeoutSeconds.IsUnknown() {
		return defaultWaitSeconds * time.Second
	}

	return time.Duration(data.WaitTimeoutSeconds.ValueInt64()) * time.Second
}

func nestedString(object map[string]interface{}, fields ...string) string {
	value, _, _ := unstructured.NestedString(object, fields...)
	return value
}
