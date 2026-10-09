package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccCatalogue runs against the real API. It only runs with TF_ACC=1 and
// HOSTEREO_API_KEY set (resource.Test skips without TF_ACC). It reads the
// public catalogue only, so it creates nothing and costs nothing.
func TestAccCatalogue(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			if os.Getenv("HOSTEREO_API_KEY") == "" {
				t.Skip("HOSTEREO_API_KEY not set")
			}
		},
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{{
			Config: `provider "hostereo" {}
data "hostereo_regions" "all" {}
data "hostereo_plans" "all" {}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet("data.hostereo_regions.all", "regions.0.id"),
				resource.TestCheckResourceAttrSet("data.hostereo_plans.all", "plans.0.id"),
			),
		}},
	})
}
