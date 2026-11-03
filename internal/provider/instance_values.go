// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"math/big"
	"strconv"
)

func attrValueToUnstructured(value attr.Value) (interface{}, error) {
	if value == nil || value.IsNull() {
		return nil, nil
	}
	if value.IsUnknown() {
		return nil, fmt.Errorf("value is unknown")
	}

	switch typed := value.(type) {
	case types.String:
		return typed.ValueString(), nil
	case types.Bool:
		return typed.ValueBool(), nil
	case types.Int64:
		return typed.ValueInt64(), nil
	case types.Float64:
		return typed.ValueFloat64(), nil
	case types.Number:
		floatValue, _ := typed.ValueBigFloat().Float64()
		return floatValue, nil
	case types.Object:
		result := make(map[string]interface{}, len(typed.Attributes()))
		for key, attribute := range typed.Attributes() {
			if attribute.IsNull() {
				continue
			}
			converted, err := attrValueToUnstructured(attribute)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			result[crdFieldName(key)] = converted
		}
		return result, nil
	case types.Map:
		result := make(map[string]interface{}, len(typed.Elements()))
		for key, element := range typed.Elements() {
			if element.IsNull() {
				continue
			}
			converted, err := attrValueToUnstructured(element)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			result[key] = converted
		}
		return result, nil
	case types.List:
		return attrValuesToUnstructured(typed.Elements())
	case types.Set:
		return attrValuesToUnstructured(typed.Elements())
	case types.Dynamic:
		return attrValueToUnstructured(typed.UnderlyingValue())
	default:
		return nil, fmt.Errorf("unsupported Terraform value type %T", value)
	}
}

func attrValuesToUnstructured(values []attr.Value) ([]interface{}, error) {
	result := make([]interface{}, 0, len(values))
	for index, value := range values {
		converted, err := attrValueToUnstructured(value)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", index, err)
		}
		result = append(result, converted)
	}
	return result, nil
}

func attrValueFromUnstructured(value interface{}, typ attr.Type) (attr.Value, error) {
	if value == nil {
		return nullAttrValue(typ), nil
	}

	switch typed := typ.(type) {
	case basetypes.StringType:
		stringValue, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("expected string, got %T", value)
		}
		return types.StringValue(stringValue), nil
	case basetypes.BoolType:
		boolValue, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("expected bool, got %T", value)
		}
		return types.BoolValue(boolValue), nil
	case basetypes.Int64Type:
		intValue, err := int64Value(value)
		if err != nil {
			return nil, err
		}
		return types.Int64Value(intValue), nil
	case basetypes.Float64Type:
		floatValue, err := float64Value(value)
		if err != nil {
			return nil, err
		}
		return types.Float64Value(floatValue), nil
	case basetypes.NumberType:
		floatValue, err := float64Value(value)
		if err != nil {
			return nil, err
		}
		return types.NumberValue(big.NewFloat(floatValue)), nil
	case types.ObjectType:
		objectValue, ok := value.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected object, got %T", value)
		}
		attributes := make(map[string]attr.Value, len(typed.AttributeTypes()))
		for name, attributeType := range typed.AttributeTypes() {
			attribute, found := objectValue[crdFieldName(name)]
			if !found {
				attributes[name] = nullAttrValue(attributeType)
				continue
			}
			converted, err := attrValueFromUnstructured(attribute, attributeType)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			attributes[name] = converted
		}
		return types.ObjectValueMust(typed.AttributeTypes(), attributes), nil
	case types.MapType:
		mapValue, ok := value.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected map, got %T", value)
		}
		elements := make(map[string]attr.Value, len(mapValue))
		for name, element := range mapValue {
			converted, err := attrValueFromUnstructured(element, typed.ElementType())
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			elements[name] = converted
		}
		return types.MapValueMust(typed.ElementType(), elements), nil
	case types.ListType:
		listValue, ok := value.([]interface{})
		if !ok {
			return nil, fmt.Errorf("expected list, got %T", value)
		}
		elements := make([]attr.Value, len(listValue))
		for index, element := range listValue {
			converted, err := attrValueFromUnstructured(element, typed.ElementType())
			if err != nil {
				return nil, fmt.Errorf("element %d: %w", index, err)
			}
			elements[index] = converted
		}
		return types.ListValueMust(typed.ElementType(), elements), nil
	case types.SetType:
		listValue, ok := value.([]interface{})
		if !ok {
			return nil, fmt.Errorf("expected set, got %T", value)
		}
		elements := make([]attr.Value, len(listValue))
		for index, element := range listValue {
			converted, err := attrValueFromUnstructured(element, typed.ElementType())
			if err != nil {
				return nil, fmt.Errorf("element %d: %w", index, err)
			}
			elements[index] = converted
		}
		return types.SetValueMust(typed.ElementType(), elements), nil
	default:
		return nil, fmt.Errorf("unsupported Terraform type %T", typ)
	}
}

func nullAttrValue(typ attr.Type) attr.Value {
	switch typed := typ.(type) {
	case basetypes.StringType:
		return types.StringNull()
	case basetypes.BoolType:
		return types.BoolNull()
	case basetypes.Int64Type:
		return types.Int64Null()
	case basetypes.Float64Type:
		return types.Float64Null()
	case basetypes.NumberType:
		return types.NumberNull()
	case types.ObjectType:
		return types.ObjectNull(typed.AttributeTypes())
	case types.MapType:
		return types.MapNull(typed.ElementType())
	case types.ListType:
		return types.ListNull(typed.ElementType())
	case types.SetType:
		return types.SetNull(typed.ElementType())
	default:
		return types.StringNull()
	}
}

func int64Value(value interface{}) (int64, error) {
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case int64:
		return typed, nil
	case float64:
		return int64(typed), nil
	case json.Number:
		return typed.Int64()
	case string:
		return strconv.ParseInt(typed, 10, 64)
	default:
		return 0, fmt.Errorf("expected integer, got %T", value)
	}
}

func float64Value(value interface{}) (float64, error) {
	switch typed := value.(type) {
	case float32:
		return float64(typed), nil
	case float64:
		return typed, nil
	case int:
		return float64(typed), nil
	case int32:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case json.Number:
		return typed.Float64()
	case string:
		return strconv.ParseFloat(typed, 64)
	default:
		return 0, fmt.Errorf("expected number, got %T", value)
	}
}
