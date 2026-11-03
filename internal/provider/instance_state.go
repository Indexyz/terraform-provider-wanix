// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"k8s.io/apimachinery/pkg/api/resource"
)

func managedMetadata(live map[string]string, prior types.Map) map[string]string {
	result := map[string]string{}
	for key := range prior.Elements() {
		if value, ok := live[key]; ok {
			result[key] = value
		}
	}
	return result
}

// The operator uses omitempty when updating its typed objects. Preserve explicit
// zero values from Terraform when those values disappear during serialization.
func restoreOmittedZeroValues(live interface{}, prior attr.Value) interface{} {
	if prior == nil || prior.IsNull() || prior.IsUnknown() {
		return live
	}
	switch p := prior.(type) {
	case types.Object:
		object, ok := live.(map[string]interface{})
		if !ok {
			return live
		}
		for key, value := range p.Attributes() {
			name := crdFieldName(key)
			if name == "cpu" {
				if previous, ok := value.(types.String); ok && !previous.IsNull() && !previous.IsUnknown() {
					if actual, ok := object[name].(string); ok {
						a, ae := resource.ParseQuantity(actual)
						b, be := resource.ParseQuantity(previous.ValueString())
						if ae == nil && be == nil && a.Cmp(b) == 0 {
							object[name] = previous.ValueString()
						}
					}
				}
			}
			existing, found := object[name]
			if !found {
				switch v := value.(type) {
				case types.Bool:
					if !v.IsNull() && !v.IsUnknown() && !v.ValueBool() {
						object[name] = false
					}
				case types.Int64:
					if !v.IsNull() && !v.IsUnknown() && v.ValueInt64() == 0 {
						object[name] = int64(0)
					}
				case types.String:
					if !v.IsNull() && !v.IsUnknown() && v.ValueString() == "" {
						object[name] = ""
					}
				case types.List:
					if !v.IsNull() && !v.IsUnknown() && len(v.Elements()) == 0 {
						object[name] = []interface{}{}
					}
				case types.Map:
					if !v.IsNull() && !v.IsUnknown() && len(v.Elements()) == 0 {
						object[name] = map[string]interface{}{}
					}
				}
			} else {
				object[name] = restoreOmittedZeroValues(existing, value)
			}
		}
	case types.Map:
		if object, ok := live.(map[string]interface{}); ok {
			for key, value := range p.Elements() {
				if existing, found := object[key]; found {
					object[key] = restoreOmittedZeroValues(existing, value)
				}
			}
		}
	case types.List:
		if list, ok := live.([]interface{}); ok {
			for i, value := range p.Elements() {
				if i < len(list) {
					list[i] = restoreOmittedZeroValues(list[i], value)
				}
			}
		}
	}
	return live
}
