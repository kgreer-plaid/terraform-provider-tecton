terraform {
  required_providers {
    tecton = {
      source = "registry.terraform.io/kgreer-plaid/tecton"
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

variable "tecton_service_account_id" {
  description = "A Tecton service account ID"
  type        = string
}

provider "tecton" {
  url     = var.tecton_url
  api_key = var.tecton_api_key
}

resource "tecton_workspace" "tf_workspace_test_dev" {
  name = "tf-workspace-test-dev"
  live = false
}

resource "tecton_workspace" "tf_workspace_test_live" {
  name = "tf-workspace-test-live"
  live = true
}

resource "tecton_access_policy" "tf_access_policy_test" {
  service_account_id = var.tecton_service_account_id
  admin              = false
  workspaces = {
    (tecton_workspace.tf_workspace_test_dev.name) : ["viewer"],
  }
}
