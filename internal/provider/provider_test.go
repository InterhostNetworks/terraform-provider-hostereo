package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

const testPassword = "Correct-Horse-9-Battery-SECRET"

func factories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"hostereo": providerserver.NewProtocol6WithError(New("test")()),
	}
}

func providerBlock(m *mockAPI) string {
	return fmt.Sprintf(`
provider "hostereo" {
  endpoint = %q
  api_key  = %q
}
`, m.Endpoint(), testAPIKey)
}

// envProviderBlock takes the key from HOSTEREO_API_KEY so it never appears in HCL.
func envProviderBlock(m *mockAPI) string {
	return fmt.Sprintf(`
provider "hostereo" {
  endpoint = %q
}
variable "root_password" {
  type      = string
  sensitive = true
}
`, m.Endpoint())
}

func instanceConfig(m *mockAPI, plan, power, extra string) string {
	return envProviderBlock(m) + fmt.Sprintf(`
resource "hostereo_instance" "web" {
  name     = "web-1"
  region   = "tlv-dc-1"
  plan_id  = %q
  image_id = "44444444-4444-4444-8444-444444444444"
  password = var.root_password
  billing  = "postpaid"
  %s
  %s
}
`, plan, power, extra)
}

const (
	planBasic = "11111111-1111-4111-8111-111111111111"
	planPro   = "22222222-2222-4222-8222-222222222222"
)

// noSecretsInState fails if the API key or the root password appear anywhere in state.
func noSecretsInState(s *terraform.State) error {
	for _, mod := range s.Modules {
		for name, rs := range mod.Resources {
			for k, v := range rs.Primary.Attributes {
				if strings.Contains(v, testAPIKey) || strings.Contains(v, testPassword) {
					return fmt.Errorf("secret found in state at %s.%s", name, k)
				}
			}
		}
	}
	return nil
}

func TestInstanceLifecycle(t *testing.T) {
	m := newMockAPI(t)
	m.ProtectNew = false // let the final destroy succeed

	t.Setenv("HOSTEREO_API_KEY", testAPIKey)
	t.Setenv("TF_VAR_root_password", testPassword)
	logPath := filepath.Join(t.TempDir(), "tf.log")
	if p := os.Getenv("HST_KEEP_LOG"); p != "" {
		logPath = p
	}
	t.Setenv("TF_LOG", "TRACE")
	t.Setenv("TF_LOG_PATH", logPath)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{ // create → poll operation → read
				Config: instanceConfig(m, planBasic, "", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("hostereo_instance.web", "status", "available"),
					resource.TestCheckResourceAttr("hostereo_instance.web", "plan_name", "Basic"),
					resource.TestCheckResourceAttr("hostereo_instance.web", "power_state", "running"),
					resource.TestCheckResourceAttr("hostereo_instance.web", "deletion_protection", "false"),
					resource.TestCheckResourceAttr("hostereo_instance.web", "region_id", "33333333-3333-4333-8333-333333333333"),
					resource.TestCheckNoResourceAttr("hostereo_instance.web", "password"),
					resource.TestCheckResourceAttrSet("hostereo_instance.web", "id"),
					noSecretsInState,
					func(*terraform.State) error {
						if len(m.CreateBodies) != 1 {
							return fmt.Errorf("want 1 create, got %d", len(m.CreateBodies))
						}
						b := m.CreateBodies[0]
						if b["password"] != testPassword || b["billing"] != "postpaid" || b["region"] != "tlv-dc-1" {
							return fmt.Errorf("create body wrong: %v", b)
						}
						if !strings.HasPrefix(m.IdemKeys[0], "terraform-") {
							return fmt.Errorf("missing Idempotency-Key: %q", m.IdemKeys[0])
						}
						if !strings.HasPrefix(m.UserAgents[0], "terraform-provider-hostereo/") {
							return fmt.Errorf("bad User-Agent %q", m.UserAgents[0])
						}
						return nil
					},
				),
			},
			{ // resize in place
				Config: instanceConfig(m, planPro, "", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("hostereo_instance.web", "plan_name", "Pro"),
					resource.TestCheckResourceAttr("hostereo_instance.web", "cpu", "4"),
					func(*terraform.State) error {
						if m.count("POST /instances/") == 0 || m.count("POST /instances") != 1+m.count("POST /instances/") {
							return fmt.Errorf("expected one create and a resize, calls: %v", m.Calls)
						}
						if m.count("POST /instances") != 2 {
							return fmt.Errorf("resize should not recreate: %v", m.Calls)
						}
						return nil
					},
				),
			},
			{ // power off in place
				Config: instanceConfig(m, planPro, `power_state = "stopped"`, ""),
				Check:  resource.TestCheckResourceAttr("hostereo_instance.web", "power_state", "stopped"),
			},
			{ // project + tags in place
				Config: instanceConfig(m, planPro, `power_state = "stopped"`, `project_id = hostereo_project.p.id
  tag_ids = [hostereo_tag.t.id]`) + `
resource "hostereo_project" "p" {
  name        = "web"
  description = "front ends"
}
resource "hostereo_tag" "t" { name = "prod" }
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("hostereo_instance.web", "project_id", "hostereo_project.p", "id"),
					resource.TestCheckResourceAttr("hostereo_instance.web", "tag_ids.#", "1"),
					resource.TestCheckResourceAttr("hostereo_project.p", "description", "front ends"),
				),
			},
			{ // import by uuid
				ResourceName:            "hostereo_instance.web",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"image_id", "billing", "password", "timeouts"},
			},
		},
	})

	// destroy sent the name as confirmation
	if len(m.DeleteBodies) == 0 || m.DeleteBodies[len(m.DeleteBodies)-1]["confirm"] != "web-1" {
		t.Fatalf("delete confirm body wrong: %v", m.DeleteBodies)
	}
	for _, i := range m.instances {
		if !i.Deleted {
			t.Fatalf("instance %s not deleted", i.ID)
		}
	}
	// the key must never be logged
	logs, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading TF_LOG output: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("TF_LOG output is empty, the leak check would prove nothing")
	}
	if strings.Contains(string(logs), testAPIKey) {
		t.Fatal("API key found in TF_LOG output")
	}
	if strings.Contains(string(logs), testPassword) {
		t.Fatal("root password found in TF_LOG output")
	}
	for _, h := range m.AuthHeaders {
		if h != "" && h != "Bearer "+testAPIKey {
			t.Fatalf("unexpected auth header %q", h)
		}
	}
}

func TestInstanceDeletionProtected(t *testing.T) {
	m := newMockAPI(t) // ProtectNew = true, like the real API
	t.Setenv("HOSTEREO_API_KEY", testAPIKey)
	t.Setenv("TF_VAR_root_password", testPassword)
	cfg := instanceConfig(m, planBasic, "", "")
	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check:  resource.TestCheckResourceAttr("hostereo_instance.web", "deletion_protection", "true"),
			},
			{ // destroy is refused with the console instructions, and nothing is flipped
				Config:      cfg,
				Destroy:     true,
				ExpectError: regexp.MustCompile(`Deletion protection is enabled for web-1\.\s+Turn\s+it\s+off\s+in\s+the\s+Hostereo\s+console\s+\(instance\s+Overview\s+→\s+API\s+delete\s+protection\),\s+then\s+run\s+destroy\s+again\.`),
			},
			{ // operator turned it off in the console → destroy works
				PreConfig: func() {
					m.mu.Lock()
					for _, i := range m.instances {
						i.Protected = false
					}
					m.mu.Unlock()
				},
				Config:  cfg,
				Destroy: true,
			},
		},
	})
	for _, c := range m.Calls {
		if strings.HasPrefix(c, "PATCH ") {
			t.Fatalf("provider must never change deletion protection, saw %s", c)
		}
	}
}

func TestDataSources(t *testing.T) {
	m := newMockAPI(t)
	m.RateLimitNth = 1 // first /regions call gets 429 + Retry-After
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{{
			Config: providerBlock(m) + `
data "hostereo_regions" "all" {}
data "hostereo_plans" "all" {}
data "hostereo_images" "tlv" { region_id = "33333333-3333-4333-8333-333333333333" }
data "hostereo_applications" "all" {}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.hostereo_regions.all", "regions.0.slug", "tlv-dc-1"),
				resource.TestCheckResourceAttr("data.hostereo_plans.all", "plans.#", "2"),
				resource.TestCheckResourceAttr("data.hostereo_images.tlv", "images.0.distribution", "Ubuntu 24.04"),
				resource.TestCheckResourceAttr("data.hostereo_applications.all", "applications.0.versions.0.id", "55555555-5555-4555-8555-555555555555"),
				func(*terraform.State) error {
					if n := m.count("GET /regions"); n < 2 {
						return fmt.Errorf("expected a retry after 429, /regions calls = %d", n)
					}
					return nil
				},
			),
		}},
	})
}

func TestProjectTagForgetOnDestroy(t *testing.T) {
	m := newMockAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: providerBlock(m) + `resource "hostereo_tag" "t" { name = "prod" }
resource "hostereo_project" "p" { name = "web" }`,
				Check: resource.TestCheckResourceAttr("hostereo_tag.t", "name", "prod"),
			},
			{
				Config: providerBlock(m) + `resource "hostereo_tag" "t" { name = "production" }
resource "hostereo_project" "p" { name = "web2" }`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("hostereo_tag.t", "name", "production"),
					resource.TestCheckResourceAttr("hostereo_project.p", "name", "web2"),
				),
			},
			{ResourceName: "hostereo_tag.t", ImportState: true, ImportStateVerify: true},
			{ResourceName: "hostereo_project.p", ImportState: true, ImportStateVerify: true},
		},
	})
	if len(m.tags) != 1 || len(m.projects) != 1 {
		t.Fatalf("destroy must not delete tags/projects in the API: %d tags, %d projects", len(m.tags), len(m.projects))
	}
}

func TestMissingAPIKey(t *testing.T) {
	m := newMockAPI(t)
	t.Setenv("HOSTEREO_API_KEY", "")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{{
			Config:      fmt.Sprintf(`provider "hostereo" { endpoint = %q }`, m.Endpoint()) + "\n" + `resource "hostereo_tag" "t" { name = "x" }`,
			ExpectError: regexp.MustCompile(`Missing Hostereo API key`),
		}},
	})
}
