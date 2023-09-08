resource "tecton_access_policy" "tf_access_policy_test" {
  service_account_id = "abc"
  admin              = false
  all_workspaces     = ["viewer"]
  workspaces = {
    "test-workspace" : ["operator", "editor"],
  }
}
