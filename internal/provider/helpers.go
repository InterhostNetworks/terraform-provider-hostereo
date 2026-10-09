package provider

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/InterhostNetworks/terraform-provider-hostereo/internal/client"
)

// clientFrom extracts the API client from provider data. requireKey adds an
// error when no API key is configured.
func clientFrom(data any, requireKey bool, diags *diag.Diagnostics) *client.Client {
	if data == nil {
		return nil // provider not configured yet (validation phase)
	}
	pd, ok := data.(*providerData)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", data))
		return nil
	}
	if requireKey && !pd.hasKey {
		diags.AddError("Missing Hostereo API key", missingKeyDetail)
		return nil
	}
	return pd.client
}

func newIdempotencyKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "terraform-" + hex.EncodeToString(b)
}

// apiErr adds an error diagnostic for an API call.
func apiErr(diags *diag.Diagnostics, summary string, err error) {
	diags.AddError(summary, err.Error())
}

type diagnosticsT = diag.Diagnostics
