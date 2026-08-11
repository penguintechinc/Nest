package webhook

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// ConversionWebhookSetup is a placeholder for future CRD conversion webhook implementation.
// This is invoked when introducing a new API version (v2, v3, etc.) that requires
// conversion logic between versions.
//
// The conversion webhook will:
// 1. Accept requests for conversion between v1 and v2 (or higher) API versions
// 2. Transform stored objects from the old version to the new version and vice versa
// 3. Handle both round-trip conversion (v1 -> v2 -> v1) and one-way conversion
//
// See crd-versioning.md for the complete versioning strategy.

// ConversionWebhook holds conversion logic for multiple API versions.
// TODO: implement conversion webhook when v2 types are introduced.
// This will require:
//  1. Defining v2 types in apis/v2/
//  2. Implementing conversion functions: ConvertFrom (v1->v2) and ConvertTo (v2->v1)
//  3. Registering the conversion with the scheme in the webhook setup
type ConversionWebhook struct{}

// SetupWithManager registers the conversion webhook with the controller manager.
// Currently a no-op; will be fully implemented when v2 types are introduced.
// TODO: implement when converting to v2 API.
func (c *ConversionWebhook) SetupWithManager(mgr manager.Manager) error {
	// This stub will be replaced with actual webhook setup:
	// - Create a conversion.ConvertFunc that routes by source/dest versions
	// - Attach conversion logic to the scheme
	// - Register with the manager webhook server
	return nil
}

// ConvertFuncFactory returns a conversion.ConvertFunc for the given types.
// TODO: implement when v2 types are introduced.
// Example signature (not implemented yet):
//   func ConvertFuncFactory(srcType, dstType string) conversion.ConvertFunc {
//       return func(srcObj conversion.Convertible, dstObj conversion.Convertible, _ interface{}) error {
//           switch {
//           case srcType == "v1" && dstType == "v2":
//               return convertV1ToV2(srcObj, dstObj)
//           case srcType == "v2" && dstType == "v1":
//               return convertV2ToV1(srcObj, dstObj)
//           default:
//               return fmt.Errorf("unsupported conversion: %s -> %s", srcType, dstType)
//           }
//       }
//   }

var _ = fmt.Sprintf("placeholder for conversion webhook implementation")
