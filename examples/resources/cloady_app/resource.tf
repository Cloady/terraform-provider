# Deploy a Git repository. Cloady detects the stack and builds it.
resource "cloady_app" "api" {
  workspace = cloady_workspace.acme.slug
  name      = "API"
  region    = "eu1"

  git = {
    repository_url = "https://github.com/acme/api"
    branch         = "main"
  }
}

# Or install a catalog application.
resource "cloady_app" "db" {
  workspace = cloady_workspace.acme.slug
  name      = "Postgres"
  region    = "eu1"
  catalog   = "postgres"

  # Volumes only ever grow. Lowering a size is rejected during planning.
  volume_sizes = { data = 20 }
}
