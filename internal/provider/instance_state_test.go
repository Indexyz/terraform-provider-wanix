// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func requireType[T any](t *testing.T, value any) T {
	t.Helper()
	typed, ok := value.(T)
	if !ok {
		t.Fatalf("unexpected value type %T", value)
	}
	return typed
}

func testInstanceModel(t *testing.T) InstanceResourceModel {
	t.Helper()
	spec, err := attrValueFromUnstructured(map[string]interface{}{
		"cpu": "0.1", "pause": false, "disableWorkload": false,
		"interfaces": map[string]interface{}{"network_setup": map[string]interface{}{"table": int64(222), "useGateway": false}},
		"agent":      map[string]interface{}{"env": map[string]interface{}{"network_setup": "unchanged"}},
	}, instanceSpecType())
	if err != nil {
		t.Fatal(err)
	}
	return InstanceResourceModel{
		Name: types.StringValue("test"), Namespace: types.StringValue("default"), Spec: requireType[types.Object](t, spec),
		Labels: types.MapNull(types.StringType), Annotations: types.MapNull(types.StringType), PodIPs: types.ListNull(types.StringType),
		WaitForReady: types.BoolValue(true), WaitTimeoutSeconds: types.Int64Value(1),
	}
}

func TestSpecConversionPreservesMapKeysAndZeroValues(t *testing.T) {
	data := testInstanceModel(t)
	obj, err := instanceObjectFromPlan(&data)
	if err != nil {
		t.Fatal(err)
	}
	spec := requireType[map[string]interface{}](t, obj.Object["spec"])
	interfaces := requireType[map[string]interface{}](t, spec["interfaces"])
	if interfaces["network_setup"] == nil {
		t.Fatal("interface map key was renamed")
	}
	agent := requireType[map[string]interface{}](t, spec["agent"])
	env := requireType[map[string]interface{}](t, agent["env"])
	if env["network_setup"] != "unchanged" {
		t.Fatal("environment map key was renamed")
	}
	spec["cpu"] = "100m"
	delete(spec, "pause")
	delete(spec, "disableWorkload")
	// A missing bool (not an explicit JSON null) is how omitempty is represented.
	iface := requireType[map[string]interface{}](t, interfaces["network_setup"])
	delete(iface, "useGateway")
	original := data.Spec
	if err := setInstanceState(&data, obj); err != nil {
		t.Fatal(err)
	}
	if !data.Spec.Equal(original) {
		t.Fatalf("operator normalization caused drift: %s", data.Spec)
	}
	spec["pause"] = true
	if err := setInstanceState(&data, obj); err != nil {
		t.Fatal(err)
	}
	if data.Spec.Equal(original) {
		t.Fatal("real remote drift was hidden")
	}
}

func TestCreateWaitFailureRetainsState(t *testing.T) {
	for _, phase := range []string{"Error", "Pending"} {
		t.Run(phase, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"apiVersion":"pop.wanix.net/v1alpha1","kind":"Instance","metadata":{"name":"test","namespace":"default"},"spec":{"interfaces":{"network_setup":{"table":222}}},"status":{"phase":%q,"message":"test failure"}}`, phase)
			}))
			defer server.Close()
			client, err := (&kubernetesProviderConfig{Host: server.URL}).dynamicClient()
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			state := tfsdk.State{Schema: instanceResourceSchema()}
			data := testInstanceModel(t)
			if diags := state.Set(ctx, &data); diags.HasError() {
				t.Fatal(diags)
			}
			r := &InstanceResource{client: client}
			resp := resource.CreateResponse{State: tfsdk.State{Schema: state.Schema}}
			r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan(state)}, &resp)
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected readiness failure")
			}
			var actual InstanceResourceModel
			if diags := resp.State.Get(ctx, &actual); diags.HasError() {
				t.Fatal(diags)
			}
			if actual.ID.ValueString() != "default/test" {
				t.Fatal("created resource was orphaned")
			}
		})
	}
}

func TestReadNotFoundRemovesStateAndForbiddenPreservesIt(t *testing.T) {
	for _, code := range []int{404, 403} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				reason := "NotFound"
				if code == 403 {
					reason = "Forbidden"
				}
				fmt.Fprintf(w, `{"apiVersion":"v1","kind":"Status","reason":%q,"code":%d}`, reason, code)
			}))
			defer server.Close()
			client, err := (&kubernetesProviderConfig{Host: server.URL}).dynamicClient()
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			state := tfsdk.State{Schema: instanceResourceSchema()}
			data := testInstanceModel(t)
			data.ID = types.StringValue("default/test")
			if diags := state.Set(ctx, &data); diags.HasError() {
				t.Fatal(diags)
			}
			resp := resource.ReadResponse{State: state}
			(&InstanceResource{client: client}).Read(ctx, resource.ReadRequest{State: state}, &resp)
			if code == 404 && !resp.State.Raw.IsNull() {
				t.Fatal("404 did not remove state")
			}
			if code == 403 && (!resp.Diagnostics.HasError() || resp.State.Raw.IsNull()) {
				t.Fatal("403 must preserve state and return an error")
			}
		})
	}
}

func TestImportRejectsInvalidID(t *testing.T) {
	for _, id := range []string{"", "name", "/name", "ns/", "ns/name/extra"} {
		resp := resource.ImportStateResponse{State: tfsdk.State{Schema: instanceResourceSchema()}}
		(&InstanceResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("accepted invalid ID %q", id)
		}
	}
}

func TestMetadataOwnership(t *testing.T) {
	old := types.MapValueMust(types.StringType, map[string]attr.Value{"old": types.StringValue("value")})
	live := map[string]interface{}{"labels": map[string]interface{}{"old": "value", "operator": "keep"}}
	if err := updateManagedMetadataMap(live, "labels", types.MapNull(types.StringType), old); err != nil {
		t.Fatal(err)
	}
	labels := fmt.Sprint(live["labels"])
	if strings.Contains(labels, "old") || !strings.Contains(labels, "operator:keep") {
		t.Fatal(labels)
	}
}
