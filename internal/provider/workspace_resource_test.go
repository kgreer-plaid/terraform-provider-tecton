package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccWorkspaceResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
resource "tecton_workspace" "tf_provider_acc_test_live" {
	name = "tf-provider-acc-test-live"
	live = true
}

resource "tecton_workspace" "tf_provider_acc_test_dev" {
	name = "tf-provider-acc-test-dev"
	live = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tecton_workspace.tf_provider_acc_test_live", "name", "tf-provider-acc-test-live"),
					resource.TestCheckResourceAttr("tecton_workspace.tf_provider_acc_test_live", "live", "true"),
					resource.TestCheckResourceAttrSet("tecton_workspace.tf_provider_acc_test_live", "id"),
					resource.TestCheckResourceAttrSet("tecton_workspace.tf_provider_acc_test_live", "last_updated"),

					resource.TestCheckResourceAttr("tecton_workspace.tf_provider_acc_test_dev", "name", "tf-provider-acc-test-dev"),
					resource.TestCheckResourceAttr("tecton_workspace.tf_provider_acc_test_dev", "live", "false"),
					resource.TestCheckResourceAttrSet("tecton_workspace.tf_provider_acc_test_dev", "id"),
					resource.TestCheckResourceAttrSet("tecton_workspace.tf_provider_acc_test_dev", "last_updated"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "tecton_workspace.tf_provider_acc_test_dev",
				ImportState:       true,
				ImportStateVerify: true,
				// The last_updated attribute does not exist in the HashiCups
				// API, therefore there is no value for it during import.
				ImportStateVerifyIgnore: []string{"last_updated"},
			},
			// Update name fails
			{
				Config: providerConfig + `
resource "tecton_workspace" "tf_provider_acc_test_dev" {
	name = "tf-provider-acc-test-dev-v2"
	live = false
}
`,
				ExpectError: regexp.MustCompile("Error Updating Workspace"),
			},
			// Update live fails
			{
				Config: providerConfig + `
resource "tecton_workspace" "tf_provider_acc_test_dev" {
	name = "tf-provider-acc-test-dev"
	live = true
}
`,
				ExpectError: regexp.MustCompile("Error Updating Workspace"),
			},
			// Duplicate workspace name fails
			{
				Config: providerConfig + `
resource "tecton_workspace" "tf_provider_acc_test_dev_dup" {
	name = "tf-provider-acc-test-dev"
	live = false
}
`,
				ExpectError: regexp.MustCompile("Failed to create Tecton workspace"),
			},
			// Invalid workspace name fails
			{
				Config: providerConfig + `
resource "tecton_workspace" "tf_provider_acc_invalid_name" {
	name = "name with spaces"
	live = false
}
`,
				ExpectError: regexp.MustCompile("Invalid Attribute Value Match"),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
