data "cloady_regions" "all" {}

output "region_ids" {
  value = [for r in data.cloady_regions.all.regions : r.id]
}
