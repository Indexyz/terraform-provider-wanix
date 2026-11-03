// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func instanceSpecType() types.ObjectType {
	attributes := instanceSpecAttribute().Attributes
	attributeTypes := make(map[string]attr.Type, len(attributes))
	for name, attribute := range attributes {
		attributeTypes[name] = attribute.GetType()
	}
	return types.ObjectType{AttrTypes: attributeTypes}
}

func instanceResourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a pop.wanix.net/v1alpha1 Instance using full Kubernetes updates instead of merge patches.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Kubernetes object ID in the form namespace/name.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the Instance custom resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"namespace": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Namespace containing the Instance custom resource.",
				Default:             stringdefault.StaticString(defaultNamespace),
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"labels": schema.MapAttribute{
				Optional:            true,
				MarkdownDescription: "Labels managed on the Instance custom resource.",
				ElementType:         types.StringType,
			},
			"annotations": schema.MapAttribute{
				Optional:            true,
				MarkdownDescription: "Annotations managed on the Instance custom resource.",
				ElementType:         types.StringType,
			},
			"spec": instanceSpecAttribute(),
			"wait_for_ready": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Wait for status.phase=Ready and a non-empty status.podIPs list after create or update. Paused instances do not require Pod IPs.",
				Default:             booldefault.StaticBool(true),
			},
			"wait_timeout_seconds": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Maximum time to wait for reconciliation and deletion.",
				Default:             int64default.StaticInt64(defaultWaitSeconds),
				Validators:          []validator.Int64{int64validator.Between(1, 86400)},
			},
			"phase": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operator-reported reconciliation phase.",
			},
			"message": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operator-reported status message.",
			},
			"pod_ips": schema.ListAttribute{
				Computed:            true,
				MarkdownDescription: "Primary IP addresses reported by the operator for workload pods.",
				ElementType:         types.StringType,
			},
		},
	}
}

func instanceSpecAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Required:            true,
		MarkdownDescription: "Desired state for the pop.wanix.net Instance.",
		Attributes: map[string]schema.Attribute{
			"interfaces":               interfaceMapAttribute(),
			"memory":                   schema.Int64Attribute{Optional: true, MarkdownDescription: "Workload memory in MiB. When omitted, the operator uses 4096 MiB per interface.", Validators: []validator.Int64{int64validator.Between(0, 4294967295)}},
			"cpu":                      schema.StringAttribute{Optional: true, Validators: []validator.String{cpuQuantityValidator{}}},
			"disable_workload":         schema.BoolAttribute{Optional: true},
			"pause":                    schema.BoolAttribute{Optional: true},
			"workload_kind":            schema.StringAttribute{Optional: true, Validators: []validator.String{stringvalidator.OneOf("Deployment", "StatefulSet")}},
			"firewall":                 firewallAttribute(),
			"image_pull_secrets":       stringListAttribute(),
			"images":                   imagesAttribute(),
			"dns":                      stringListAttribute(),
			"pod_template_labels":      stringMapAttribute(),
			"pod_template_annotations": stringMapAttribute(),
			"agent":                    agentAttribute(),
		},
	}
}

func interfaceMapAttribute() schema.MapNestedAttribute {
	return schema.MapNestedAttribute{
		Required: true,
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"default":        schema.BoolAttribute{Optional: true},
			"table":          schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.Between(0, 4294967295)}},
			"address":        schema.StringAttribute{Optional: true},
			"gateway":        schema.StringAttribute{Optional: true},
			"use_gateway":    schema.BoolAttribute{Optional: true},
			"vlan":           schema.Int64Attribute{Optional: true},
			"routes":         routeListAttribute(),
			"tunnels":        tunnelListAttribute(),
			"virtual_device": schema.BoolAttribute{Optional: true},
		}},
	}
}

func routeListAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional: true,
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"destination": schema.StringAttribute{Required: true},
			"gateway":     schema.StringAttribute{Optional: true},
		}},
	}
}

func tunnelListAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional: true,
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"name":      schema.StringAttribute{Required: true},
			"local":     schema.StringAttribute{Optional: true},
			"remote":    schema.StringAttribute{Optional: true},
			"type":      schema.StringAttribute{Optional: true, Validators: []validator.String{stringvalidator.OneOf("ipip", "gre", "wireguard")}},
			"mtu":       schema.Int64Attribute{Optional: true},
			"wireguard": wireGuardAttribute(),
		}},
	}
}

func wireGuardAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true,
		Attributes: map[string]schema.Attribute{
			"port":        schema.Int64Attribute{Optional: true},
			"mtu":         schema.Int64Attribute{Optional: true},
			"private_key": schema.StringAttribute{Optional: true, Sensitive: true},
			"peers":       peerListAttribute(),
		},
	}
}

func peerListAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional: true,
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"public_key":           schema.StringAttribute{Required: true},
			"allowed_ips":          schema.ListAttribute{Required: true, ElementType: types.StringType, Validators: []validator.List{listvalidator.SizeAtLeast(1)}},
			"persistent_keepalive": schema.Int64Attribute{Optional: true},
			"endpoint":             schema.StringAttribute{Optional: true},
		}},
	}
}

func firewallAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true,
		Attributes: map[string]schema.Attribute{
			"masquerade_interfaces": stringListAttribute(),
			"port_forwarding":       portForwardingListAttribute(),
		},
	}
}

func portForwardingListAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional: true,
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"proto":  schema.StringAttribute{Optional: true},
			"from":   schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.Between(1, 65535)}},
			"to":     schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.Between(1, 65535)}},
			"target": schema.StringAttribute{Required: true},
		}},
	}
}

func imagesAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true,
		Attributes: map[string]schema.Attribute{
			"agent":         schema.StringAttribute{Optional: true},
			"core":          schema.StringAttribute{Optional: true},
			"network_setup": schema.StringAttribute{Optional: true},
			"pause":         schema.StringAttribute{Optional: true},
		},
	}
}

func agentAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true,
		Attributes: map[string]schema.Attribute{
			"env": stringMapAttribute(),
		},
	}
}

func stringListAttribute() schema.ListAttribute {
	return schema.ListAttribute{Optional: true, ElementType: types.StringType}
}

func stringMapAttribute() schema.MapAttribute {
	return schema.MapAttribute{Optional: true, ElementType: types.StringType}
}
