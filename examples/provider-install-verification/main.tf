terraform {
  required_providers {
    tecton = {
      source = "registry.terraform.io/kgreer-plaid/tecton"
    }
  }
}

provider "tecton" {
  url     = "https://yourcluster.tecton.ai"
  api_key = "abc"
}
