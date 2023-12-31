---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "tecton_workspace Resource - terraform-provider-tecton"
subcategory: ""
description: |-
  
---

# tecton_workspace (Resource)



## Example Usage

```terraform
resource "tecton_workspace" "tf_workspace_test_dev" {
  name = "tf-workspace-test-dev"
  live = false
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `live` (Boolean) True if this workspace is a live workspace. False otherwise (i.e. it is a development workspace)
- `name` (String) The name of the workspace.

### Read-Only

- `id` (String) Identifier for this workspace. Equal to the workspace name.
- `last_updated` (String)

## Import

Import is supported using the following syntax:

```shell
# Workspaces can be imported by specifying the workspace name
terraform import tecton_workspace.example test-workspace-name
```
