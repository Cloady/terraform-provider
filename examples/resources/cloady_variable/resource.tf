resource "cloady_variable" "database_url" {
  workspace = cloady_workspace.acme.slug
  app       = cloady_app.api.slug
  region    = cloady_app.api.region
  key       = "DATABASE_URL"
  value     = var.database_url
  is_secret = true
}
