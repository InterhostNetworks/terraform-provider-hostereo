terraform {
  required_providers {
    hostereo = {
      source  = "hostereo/hostereo"
      version = "~> 0.1"
    }
  }
}

# The API key is read from HOSTEREO_API_KEY. Keys are IP-bound: allow the
# address Terraform runs from (Hostereo console → Account → API Keys).
provider "hostereo" {}
