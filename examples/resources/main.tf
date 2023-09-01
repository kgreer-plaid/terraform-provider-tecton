terraform {
  required_providers {
    tecton = {
      source = "registry.terraform.io/k-greer/tecton"
    }
  }
}

variable "tecton_api_key" {
  description = "API Key for the Tecton provider."
  type        = string
  sensitive   = true
}

variable "tecton_url" {
  description = "The URL for your Tecton Cluster. For example, https://yourcluster.tecton.ai"
  type        = string
}

provider "tecton" {
  url     = var.tecton_url
  api_key = var.tecton_api_key
}

resource "tecton_workspace" "tf_workspace_test_sept1_v2" {
  name = "tf-workspace-test-sept1"
  live = false
}
