# Terraform Provider for Hostereo

Manage [Hostereo](https://www.hostereo.com) VPS instances, projects and tags with Terraform, through the
Hostereo External API (`https://api-public.hostereo.com/api/v1`).

- **Resources:** `hostereo_instance`, `hostereo_project`, `hostereo_tag`
- **Data sources:** `hostereo_regions`, `hostereo_plans`, `hostereo_images`, `hostereo_applications`, `hostereo_instance`

Full documentation is in [`docs/`](docs/) (rendered on the Terraform Registry).

## Usage

```hcl
terraform {
  required_providers {
    hostereo = {
      source  = "interhostnetworks/hostereo"
      version = "~> 0.1"
    }
  }
}

provider "hostereo" {} # reads HOSTEREO_API_KEY

variable "root_password" {
  type      = string
  sensitive = true
}

resource "hostereo_instance" "web" {
  name     = "web-1"
  region   = "tlv-dc-1"
  plan_id  = "…"   # from data.hostereo_plans
  image_id = "…"   # from data.hostereo_images
  password = var.root_password  # write-only, never stored in state
  billing  = "postpaid"
}
```

Requirements: Terraform **1.11+** (the instance `password` is a write-only argument).

### API keys

Create a key in the Hostereo console under **Account → API Keys**. Keys are bound to source IPs: allow the
address Terraform (or your CI runner) uses. Scopes: `instances:read`, `instances:create`, `instances:write`,
`operations:read`, and `instances:delete` for destroying instances.

### Deletion protection

Every instance is deletion-protected by default on the server. The provider shows it as the read-only
`deletion_protection` attribute and **never changes it**. To destroy an instance, turn protection off in the
Hostereo console (instance **Overview → API delete protection**), then run `terraform destroy`. Until then
destroy fails with that instruction.

### Behaviour notes

| Change | What happens |
|---|---|
| `plan_id` | resize in place (VM hard-restarts; disks never shrink) |
| `power_state` | start / stop in place |
| `project_id`, `tag_ids` | updated in place |
| `name`, `region`, `image_id`, `application_version_id`, `app_fields` | replace the instance |
| `password`, `ssh_key_ids`, `billing` | creation-time only; later changes are not applied |
| destroy `hostereo_project` / `hostereo_tag` | removed from state only (the API has no delete) |

## Development

```sh
go build -o terraform-provider-hostereo
go test ./...                  # unit tests incl. full Terraform runs against a mock API
TF_ACC=1 HOSTEREO_API_KEY=… go test ./... -run TestAcc   # read-only acceptance test against the real API
go generate ./...              # regenerate docs/ with tfplugindocs
```

Use a local build without publishing via `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "interhostnetworks/hostereo" = "/path/to/terraform-provider-hostereo"
  }
  direct {}
}
```

## Releasing

Push a `vX.Y.Z` tag. GitHub Actions (`.github/workflows/release.yml`) builds all platforms with GoReleaser,
signs `SHA256SUMS` with the GPG key from the `GPG_PRIVATE_KEY` / `PASSPHRASE` secrets and creates the GitHub
release, which the Terraform Registry ingests.

## License

MPL-2.0
