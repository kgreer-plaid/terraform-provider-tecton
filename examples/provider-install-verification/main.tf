terraform {
  required_providers {
    tecton = {
      source = "registry.terraform.io/k-greer/tecton"
    }
  }
}

provider "tecton" {
  url     = "https://yourcluster.tecton.ai"
  api_key = "test"
}

