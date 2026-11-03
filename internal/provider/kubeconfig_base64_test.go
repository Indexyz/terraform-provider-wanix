// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const inlineKubeconfig = `apiVersion: v1
kind: Config
current-context: first
clusters:
- name: first
  cluster:
    server: https://first.example.com
- name: second
  cluster:
    server: https://second.example.com
users:
- name: user
  user:
    token: inline-test-token
contexts:
- name: first
  context:
    cluster: first
    user: user
- name: second
  context:
    cluster: second
    user: user
`

func TestBase64Kubeconfig(t *testing.T) {
	// Inline configuration must not fall back to an unrelated kubeconfig file.
	t.Setenv("KUBECONFIG", "/does-not-exist/config")
	encoded := base64.StdEncoding.EncodeToString([]byte(inlineKubeconfig))
	for _, test := range []struct{ context, host string }{{"", "https://first.example.com"}, {"second", "https://second.example.com"}} {
		t.Run(test.host, func(t *testing.T) {
			cfg, err := (&kubernetesProviderConfig{KubeconfigBase64: encoded, ConfigContext: test.context}).restConfig()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Host != test.host || cfg.BearerToken != "inline-test-token" {
				t.Fatal("wrong inline context or credentials")
			}
		})
	}
}

func TestInvalidBase64KubeconfigDoesNotExposeContents(t *testing.T) {
	for _, input := range []string{
		"not base64!",
		base64.StdEncoding.EncodeToString([]byte("secret-test-token: [malformed")),
		base64.StdEncoding.EncodeToString([]byte("apiVersion: v1\nkind: Config\n")),
	} {
		_, err := (&kubernetesProviderConfig{KubeconfigBase64: input}).restConfig()
		if err == nil {
			t.Fatal("expected invalid configuration error")
		}
		if strings.Contains(err.Error(), "secret-test-token") || strings.Contains(err.Error(), input) {
			t.Fatal("error exposed kubeconfig contents")
		}
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(inlineKubeconfig))
	if _, err := (&kubernetesProviderConfig{KubeconfigBase64: encoded, ConfigContext: "missing"}).restConfig(); err == nil {
		t.Fatal("missing context was accepted")
	}
}

func TestProviderBase64Configuration(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(inlineKubeconfig))
	for _, test := range []struct {
		name      string
		overrides map[string]attr.Value
		wantError bool
	}{
		{name: "inline", overrides: map[string]attr.Value{"kubeconfig_base64": types.StringValue(encoded)}},
		{name: "context", overrides: map[string]attr.Value{"kubeconfig_base64": types.StringValue(encoded), "config_context": types.StringValue("second")}},
		{name: "path conflict", overrides: map[string]attr.Value{"kubeconfig_base64": types.StringValue(encoded), "config_path": types.StringValue("/config")}, wantError: true},
		{name: "host conflict", overrides: map[string]attr.Value{"kubeconfig_base64": types.StringValue(encoded), "host": types.StringValue("https://cluster.example.com")}, wantError: true},
		{name: "in-cluster conflict", overrides: map[string]attr.Value{"kubeconfig_base64": types.StringValue(encoded), "in_cluster_config": types.BoolValue(true)}, wantError: true},
		{name: "credential conflict", overrides: map[string]attr.Value{"kubeconfig_base64": types.StringValue(encoded), "token": types.StringValue("extra-token")}, wantError: true},
		{name: "empty", overrides: map[string]attr.Value{"kubeconfig_base64": types.StringValue("")}, wantError: true},
		{name: "unknown", overrides: map[string]attr.Value{"kubeconfig_base64": types.StringUnknown()}, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			p := &WanixProvider{}
			sr := provider.SchemaResponse{}
			p.Schema(ctx, provider.SchemaRequest{}, &sr)
			schemaType := requireType[types.ObjectType](t, sr.Schema.Type())
			attrTypes := schemaType.AttributeTypes()
			values := map[string]attr.Value{}
			for name, typ := range attrTypes {
				values[name] = nullAttrValue(typ)
			}
			for name, value := range test.overrides {
				values[name] = value
			}
			raw, err := types.ObjectValueMust(attrTypes, values).ToTerraformValue(ctx)
			if err != nil {
				t.Fatal(err)
			}
			resp := provider.ConfigureResponse{}
			p.Configure(ctx, provider.ConfigureRequest{Config: tfsdk.Config{Raw: raw, Schema: sr.Schema}}, &resp)
			if resp.Diagnostics.HasError() != test.wantError {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
			if !test.wantError {
				cfg, ok := resp.ResourceData.(*kubernetesProviderConfig)
				if !ok || cfg.KubeconfigBase64 != encoded {
					t.Fatal("inline kubeconfig not passed to resource")
				}
			}
			if !sr.Schema.Attributes["kubeconfig_base64"].IsSensitive() {
				t.Fatal("kubeconfig_base64 must be sensitive")
			}
		})
	}
}
