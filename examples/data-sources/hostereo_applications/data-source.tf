data "hostereo_applications" "all" {}

output "wordpress_versions" {
  value = one([for a in data.hostereo_applications.all.applications : a.versions if a.name == "WordPress"])
}
