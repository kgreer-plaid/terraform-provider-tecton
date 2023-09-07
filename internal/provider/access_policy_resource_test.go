package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAccessPolicyResource_validation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// No user_id or service_account_id fails
			{
				Config: providerConfig + `
resource "tecton_access_policy" "no_id" {
	admin = false
}
`,
				ExpectError: regexp.MustCompile("Missing Attribute Configuration"),
			},
			// Both user_id or service_account_id fails
			{
				Config: providerConfig + `
resource "tecton_access_policy" "both_ids" {
	user_id = "test"
	service_account_id = "test"
	admin = false
}
`,
				ExpectError: regexp.MustCompile("Invalid Attribute Combination"),
			},
			// No access policies fails
			{
				Config: providerConfig + `
resource "tecton_access_policy" "no_access_policies" {
	user_id = "test"
}
`,
				ExpectError: regexp.MustCompile("Missing Attribute Configuration"),
			},
			// Invalid all_workspaces role fails
			{
				Config: providerConfig + `
resource "tecton_access_policy" "invalid_all_workspaces_role" {
	user_id = "test"
	all_workspaces = ["test"]
}
`,
				ExpectError: regexp.MustCompile("Invalid Attribute Value Match"),
			},
			// Invalid workspace role fails
			{
				Config: providerConfig + `
resource "tecton_access_policy" "invalid_workspace_role" {
	user_id = "test"
	workspaces = {
		"test": ["test"]
	}
}
`,
				ExpectError: regexp.MustCompile("Invalid Attribute Value Match"),
			},
			// Duplicate roles in workspaces
			{
				Config: providerConfig + `
resource "tecton_access_policy" "dup_roles_workspaces" {
	user_id = "invalid-user"
	workspaces = {
		"test" : ["viewer", "viewer"]
	}
}
`,
				ExpectError: regexp.MustCompile("Duplicate List Value"),
			},
			// Duplicate roles in workspaces
			{
				Config: providerConfig + `
resource "tecton_access_policy" "dup_roles_all_workspaces" {
	user_id = "invalid-user"
	all_workspaces = ["viewer", "viewer"]
}
`,
				ExpectError: regexp.MustCompile("Duplicate List Value"),
			},
			// Invalid user fails
			{
				Config: providerConfig + `
resource "tecton_access_policy" "invalid_user" {
	user_id = "invalid-user"
	workspaces = {
		"test" : ["viewer"]
	}
}
`,
				ExpectError: regexp.MustCompile("Role Read Failure"),
			},
			// Invalid service account fails
			{
				Config: providerConfig + `
resource "tecton_access_policy" "invalid_service_account" {
	service_account_id = "invalidservice"
	workspaces = {
		"test": ["viewer"]
	}
}
`,
				ExpectError: regexp.MustCompile("Role Read Failure"),
			},
			// Invalid workspace fails
			{
				Config: providerConfig + `
resource "tecton_access_policy" "invalid_workspace" {
	service_account_id = var.tecton_service_account_no_existing_roles
	workspaces = {
		"invalid-workspace": ["viewer"]
	}
}
`,
				ExpectError: regexp.MustCompile("Access Policy Creation Failure"),
			},
			// Create fails when access policy already exists
			{
				Config: providerConfig + `
resource "tecton_access_policy" "existing_roles" {
	service_account_id = var.tecton_service_account_existing_roles
	workspaces = {
		"existing-role-workspace": ["viewer"]
	}
}
`,
				ExpectError: regexp.MustCompile("Access Policy Already Exists"),
			},
			// I'd also like to test the following case(s), but not sure how to do it using this framework
			// Import state invalid ID
		},
	})
}

func TestAccAccessPolicyResource_crud(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Normal create, all fields
			{
				Config: providerConfig + `
resource "tecton_workspace" "tf_provider_acc_test_dev_1" {
	name = "tf-provider-acc-test-dev-1"
	live = false
}

resource "tecton_workspace" "tf_provider_acc_test_dev_2" {
	name = "tf-provider-acc-test-dev-2"
	live = false
}

resource "tecton_access_policy" "no_existing_roles" {
	service_account_id = var.tecton_service_account_no_existing_roles
	admin = true
	workspaces = {
		(tecton_workspace.tf_provider_acc_test_dev_1.name): ["viewer", "editor"],
		(tecton_workspace.tf_provider_acc_test_dev_2.name): ["operator"]
	}
	all_workspaces = ["viewer"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("tecton_access_policy.no_existing_roles", "id", regexp.MustCompile("service-*")),
					resource.TestCheckResourceAttrSet("tecton_access_policy.no_existing_roles", "last_updated"),
					resource.TestCheckNoResourceAttr("tecton_access_policy.no_existing_roles", "user_id"),
					resource.TestCheckResourceAttrSet("tecton_access_policy.no_existing_roles", "service_account_id"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "admin", "true"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "all_workspaces.#", "1"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "all_workspaces.0", "viewer"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "workspaces.%", "2"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "workspaces.tf-provider-acc-test-dev-1.#", "2"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "workspaces.tf-provider-acc-test-dev-1.0", "viewer"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "workspaces.tf-provider-acc-test-dev-1.1", "editor"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "workspaces.tf-provider-acc-test-dev-2.#", "1"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "workspaces.tf-provider-acc-test-dev-2.0", "operator"),
				),
			},
			// Duplicate ID fails
			{
				Config: providerConfig + `
resource "tecton_workspace" "tf_provider_acc_test_dev_1" {
	name = "tf-provider-acc-test-dev-1"
	live = false
}

resource "tecton_workspace" "tf_provider_acc_test_dev_2" {
	name = "tf-provider-acc-test-dev-2"
	live = false
}

resource "tecton_access_policy" "no_existing_roles_dup" {
	service_account_id = var.tecton_service_account_no_existing_roles
	admin = false
	workspaces = {
		(tecton_workspace.tf_provider_acc_test_dev_1.name): ["operator"],
	}
}
`,
				ExpectError: regexp.MustCompile("Access Policy Already Exists"),
			},
			// Update
			{
				Config: providerConfig + `
resource "tecton_workspace" "tf_provider_acc_test_dev_1" {
	name = "tf-provider-acc-test-dev-1"
	live = false
}

resource "tecton_workspace" "tf_provider_acc_test_dev_2" {
	name = "tf-provider-acc-test-dev-2"
	live = false
}

resource "tecton_access_policy" "no_existing_roles" {
	service_account_id = var.tecton_service_account_no_existing_roles
	admin = false
	workspaces = {
		(tecton_workspace.tf_provider_acc_test_dev_1.name): ["operator"],
	}
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("tecton_access_policy.no_existing_roles", "id", regexp.MustCompile("service-*")),
					resource.TestCheckResourceAttrSet("tecton_access_policy.no_existing_roles", "last_updated"),
					resource.TestCheckNoResourceAttr("tecton_access_policy.no_existing_roles", "user_id"),
					resource.TestCheckResourceAttrSet("tecton_access_policy.no_existing_roles", "service_account_id"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "admin", "false"),
					resource.TestCheckNoResourceAttr("tecton_access_policy.no_existing_roles", "all_workspaces"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "workspaces.%", "1"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "workspaces.tf-provider-acc-test-dev-1.#", "1"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "workspaces.tf-provider-acc-test-dev-1.0", "operator"),
				),
			},
			// Update again with different field configurations
			{
				Config: providerConfig + `
resource "tecton_access_policy" "no_existing_roles" {
	service_account_id = var.tecton_service_account_no_existing_roles
	admin = false
	all_workspaces = ["viewer", "editor"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("tecton_access_policy.no_existing_roles", "id", regexp.MustCompile("service-*")),
					resource.TestCheckResourceAttrSet("tecton_access_policy.no_existing_roles", "last_updated"),
					resource.TestCheckNoResourceAttr("tecton_access_policy.no_existing_roles", "user_id"),
					resource.TestCheckResourceAttrSet("tecton_access_policy.no_existing_roles", "service_account_id"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "admin", "false"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "all_workspaces.#", "2"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "all_workspaces.0", "viewer"),
					resource.TestCheckResourceAttr("tecton_access_policy.no_existing_roles", "all_workspaces.1", "editor"),
					resource.TestCheckNoResourceAttr("tecton_access_policy.no_existing_roles", "workspaces"),
				),
			},
			// Import state for service account
			{
				ResourceName:      "tecton_access_policy.no_existing_roles",
				ImportState:       true,
				ImportStateVerify: true,
				// The last_updated attribute does not exist in the HashiCups
				// API, therefore there is no value for it during import.
				ImportStateVerifyIgnore: []string{"last_updated"},
			},
			// Delete testing automatically occurs in TestCase
		},
		// This could check that all the resources are really deleted from Tecton.
		// It would be nice to implement, but Hashicorp's documentation is pretty bad for it,
		// so I can't figure out how to access the provider's configuration from this function.
		CheckDestroy: nil,
	})
}
