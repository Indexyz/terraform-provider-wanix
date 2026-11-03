// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &WanixProvider{}

type WanixProvider struct {
	version string
}

type WanixProviderModel struct {
	ConfigPath           types.String `tfsdk:"config_path"`
	KubeconfigBase64     types.String `tfsdk:"kubeconfig_base64"`
	ConfigContext        types.String `tfsdk:"config_context"`
	Host                 types.String `tfsdk:"host"`
	Token                types.String `tfsdk:"token"`
	ClientCertificate    types.String `tfsdk:"client_certificate"`
	ClientKey            types.String `tfsdk:"client_key"`
	ClusterCACertificate types.String `tfsdk:"cluster_ca_certificate"`
	InClusterConfig      types.Bool   `tfsdk:"in_cluster_config"`
}

func (p *WanixProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "wanix"
	resp.Version = p.version
}

func (p *WanixProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages SD-WAN Instance custom resources in Kubernetes without server-side apply patches.",
		Attributes: map[string]schema.Attribute{
			"config_path": schema.StringAttribute{
				MarkdownDescription: "Path to a kubeconfig file. When no connection mode is configured, uses KUBECONFIG or ~/.kube/config.",
				Optional:            true,
			},
			"kubeconfig_base64": schema.StringAttribute{
				MarkdownDescription: "Base64-encoded kubeconfig loaded in memory. Cannot be combined with config_path, host, or in_cluster_config. Supports config_context. Prefer embedded certificate and key data over file references.",
				Optional:            true,
				Sensitive:           true,
			},
			"config_context": schema.StringAttribute{
				MarkdownDescription: "Kubeconfig context to use. Defaults to the current context.",
				Optional:            true,
			},
			"host": schema.StringAttribute{
				MarkdownDescription: "Kubernetes API server URL. When set, uses the explicit credentials below instead of kubeconfig.",
				Optional:            true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "Bearer token used with host.",
				Optional:            true,
				Sensitive:           true,
			},
			"client_certificate": schema.StringAttribute{
				MarkdownDescription: "PEM-encoded client certificate used with host.",
				Optional:            true,
				Sensitive:           true,
			},
			"client_key": schema.StringAttribute{
				MarkdownDescription: "PEM-encoded client private key used with host.",
				Optional:            true,
				Sensitive:           true,
			},
			"cluster_ca_certificate": schema.StringAttribute{
				MarkdownDescription: "PEM-encoded cluster CA certificate used with host.",
				Optional:            true,
			},
			"in_cluster_config": schema.BoolAttribute{
				MarkdownDescription: "Use the pod's service account credentials. Cannot be combined with host, config_path, or config_context.",
				Optional:            true,
			},
		},
	}
}

func (p *WanixProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data WanixProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	values := map[string]types.String{
		"config_path":            data.ConfigPath,
		"kubeconfig_base64":      data.KubeconfigBase64,
		"config_context":         data.ConfigContext,
		"host":                   data.Host,
		"token":                  data.Token,
		"client_certificate":     data.ClientCertificate,
		"client_key":             data.ClientKey,
		"cluster_ca_certificate": data.ClusterCACertificate,
	}
	for name, value := range values {
		if value.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Unknown Kubernetes Configuration", "Kubernetes connection settings must be known before applying Instance resources.")
		}
	}
	if data.InClusterConfig.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("in_cluster_config"), "Unknown Kubernetes Configuration", "in_cluster_config must be known before applying Instance resources.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.KubeconfigBase64.IsNull() && (data.KubeconfigBase64.ValueString() == "" || data.ConfigPath.ValueString() != "" || data.Host.ValueString() != "" || data.InClusterConfig.ValueBool()) {
		resp.Diagnostics.AddAttributeError(path.Root("kubeconfig_base64"), "Invalid Kubernetes Configuration", "kubeconfig_base64 must not be empty and cannot be combined with config_path, host, or in_cluster_config.")
		return
	}

	if data.InClusterConfig.ValueBool() && (data.Host.ValueString() != "" || data.ConfigPath.ValueString() != "" || data.ConfigContext.ValueString() != "") {
		resp.Diagnostics.AddError("Conflicting Kubernetes Configuration", "in_cluster_config cannot be combined with host, config_path, or config_context.")
		return
	}

	if data.Host.ValueString() != "" && (data.ConfigPath.ValueString() != "" || data.ConfigContext.ValueString() != "") {
		resp.Diagnostics.AddError("Conflicting Kubernetes Configuration", "host cannot be combined with config_path or config_context.")
		return
	}
	if data.Host.ValueString() == "" && (data.Token.ValueString() != "" || data.ClientCertificate.ValueString() != "" || data.ClientKey.ValueString() != "" || data.ClusterCACertificate.ValueString() != "") {
		resp.Diagnostics.AddError("Missing Kubernetes Host", "Explicit credentials require host. For kubeconfig or in-cluster authentication, omit the explicit credential attributes.")
		return
	}

	resp.ResourceData = &kubernetesProviderConfig{
		ConfigPath:           data.ConfigPath.ValueString(),
		KubeconfigBase64:     data.KubeconfigBase64.ValueString(),
		ConfigContext:        data.ConfigContext.ValueString(),
		Host:                 data.Host.ValueString(),
		Token:                data.Token.ValueString(),
		ClientCertificate:    data.ClientCertificate.ValueString(),
		ClientKey:            data.ClientKey.ValueString(),
		ClusterCACertificate: data.ClusterCACertificate.ValueString(),
		InClusterConfig:      data.InClusterConfig.ValueBool(),
	}
}

func (p *WanixProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewInstanceResource}
}

func (p *WanixProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &WanixProvider{version: version}
	}
}
