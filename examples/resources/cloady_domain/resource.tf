resource "cloady_domain" "api" {
  workspace  = cloady_workspace.acme.slug
  app        = cloady_app.api.slug
  region     = cloady_app.api.region
  host       = "api.acme.com"
  is_primary = true
}
