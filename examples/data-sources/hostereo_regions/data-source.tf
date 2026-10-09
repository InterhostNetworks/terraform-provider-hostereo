data "hostereo_regions" "all" {}

output "region_slugs" {
  value = [for r in data.hostereo_regions.all.regions : r.slug]
}
