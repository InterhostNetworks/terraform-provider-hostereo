data "hostereo_plans" "all" {}

output "plans" {
  value = { for p in data.hostereo_plans.all.plans : p.name => p.postpaid_monthly }
}
