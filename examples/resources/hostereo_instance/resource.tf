variable "root_password" {
  type      = string
  sensitive = true
}

data "hostereo_regions" "all" {}
data "hostereo_plans" "all" {}
data "hostereo_images" "tlv" {
  region_id = one([for r in data.hostereo_regions.all.regions : r.id if r.slug == "tlv-dc-1"])
}

locals {
  ubuntu = one([for i in data.hostereo_images.tlv.images : i.id if i.distribution == "Ubuntu 24.04"])
  plan   = one([for p in data.hostereo_plans.all.plans : p.id if p.name == "Basic"])
}

resource "hostereo_project" "web" {
  name = "web"
}

resource "hostereo_instance" "web" {
  name     = "web-1"
  region   = "tlv-dc-1"
  plan_id  = local.plan
  image_id = local.ubuntu

  # Write-only: sent once at creation, never stored in state (Terraform >= 1.11).
  password = var.root_password
  billing  = "postpaid"

  project_id  = hostereo_project.web.id
  power_state = "running"

  # Deletion protection is on for every new instance and can only be turned off
  # in the Hostereo console. Until then `terraform destroy` fails on purpose.

  # On destroy: keep backups (default true; kept backups stay billed) and take
  # no final snapshot (default false).
  keep_backups_on_destroy = true
  snapshot_on_destroy     = false
}

output "ip" {
  value = hostereo_instance.web.ipv4
}

output "deletion_protection" {
  value = hostereo_instance.web.deletion_protection
}
