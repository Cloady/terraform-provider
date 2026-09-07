terraform {
  required_providers {
    cloady = {
      source  = "cloady/cloady"
      version = "~> 0.1"
    }
  }
}

# The token defaults to CLOADY_TOKEN. Mint one in the dashboard under
# Account -> Tokens. Creating workspaces or changing billing needs full scope;
# everything else works with a read token.
provider "cloady" {}
