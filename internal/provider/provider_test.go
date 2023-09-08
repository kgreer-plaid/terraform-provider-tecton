// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

const (
	// providerConfig is a shared configuration to combine with the actual
	// test configuration so the Tecton client is properly configured.
	providerConfig = `
variable "tecton_api_key" {
	description = "API Key for the Tecton provider."
	type = string
	sensitive = true
}

variable "tecton_url" {
	description = "The URL for your Tecton Cluster. For example, https://yourcluster.tecton.ai"
	type = string
}

variable "tecton_service_account_existing_roles" {
	description = "A service account ID for a service that already has an existing role"
	type = string
}

variable "tecton_service_account_no_existing_roles" {
	description = "A service account ID for a service that has no existing roles"
	type = string
}

provider "tecton" {
	url = var.tecton_url
	api_key = var.tecton_api_key
}
`
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"tecton": providerserver.NewProtocol6WithError(New("test")()),
}
