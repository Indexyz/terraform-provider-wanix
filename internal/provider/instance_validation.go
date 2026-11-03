// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"k8s.io/apimachinery/pkg/api/resource"
)

type cpuQuantityValidator struct{}

func (cpuQuantityValidator) Description(context.Context) string {
	return "Must be a non-negative Kubernetes CPU quantity, such as 2 or 100m."
}
func (v cpuQuantityValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (cpuQuantityValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	quantity, err := resource.ParseQuantity(req.ConfigValue.ValueString())
	if err != nil || quantity.Sign() < 0 {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid CPU Quantity", "Use a non-negative Kubernetes quantity, such as \"2\", \"0.1\", or \"100m\".")
	}
}
