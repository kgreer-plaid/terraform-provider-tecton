package provider

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &workspaceResource{}
	_ resource.ResourceWithConfigure   = &workspaceResource{}
	_ resource.ResourceWithImportState = &workspaceResource{}
)

// NewWorkspaceResource is a helper function to simplify the provider implementation.
func NewWorkspaceResource() resource.Resource {
	return &workspaceResource{}
}

// workspaceResource is the resource implementation.
type workspaceResource struct {
	CommandEnv    []string
	WorkspaceData Workspaces
}

// workspaceResourceModel maps the resource schema data.
type workspaceResourceModel struct {
	ID          types.String `tfsdk:"id"`
	LastUpdated types.String `tfsdk:"last_updated"`
	Name        types.String `tfsdk:"name"`
	Live        types.Bool   `tfsdk:"live"`
}

// Configure adds the provider configured client to the resource.
func (r *workspaceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(ProviderData)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected ProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.CommandEnv = providerData.CommandEnv
	r.WorkspaceData = providerData.WorkspaceData
}

// Metadata returns the resource type name.
func (r *workspaceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workspace"
}

// Schema defines the schema for the resource.
func (r *workspaceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Identifier for this workspace. Equal to the workspace name.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"last_updated": schema.StringAttribute{
				Computed: true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the workspace.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z0-9-_]+$`),
						"must contain only alphanumeric characters, hyphens, or dashes",
					),
				},
			},
			"live": schema.BoolAttribute{
				Description: "True if this workspace is a live workspace. False otherwise (i.e. it is a development workspace)",
				Required:    true,
			},
		},
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *workspaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan workspaceResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create new workspace. The name should already be validated.
	var liveArg string
	if plan.Live.ValueBool() {
		liveArg = "--live"
	} else {
		liveArg = "--no-live"
	}
	// This will automatically make the TF service account an owner of the workspace, but that's fine since it's an admin anyway.
	var cmd = exec.Command("tecton", "workspace", "create", plan.Name.ValueString(), liveArg)
	cmd.Env = r.CommandEnv
	tflog.Info(ctx, fmt.Sprintf("Creating workspace '%v'", plan.Name.ValueString()))

	output, err := cmd.CombinedOutput()
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create Tecton workspace",
			fmt.Sprintf(
				"Command to create Tecton workspace '%v' failed.\nError: %v\nOutput: %v",
				plan.Name.ValueString(),
				err.Error(),
				string(output),
			),
		)
		return
	}

	// Generated computed values
	plan.ID = plan.Name
	plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC850)) // Time format copy-pasted from Hashicorp tutorial

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *workspaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state workspaceResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If we imported this workspace the name will be empty.
	if state.Name.ValueString() == "" {
		state.Name = state.ID
	}

	// Get workspace values from prefetched list
	isLive, err := GetWorkspace(ctx, r.WorkspaceData, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Workspace", err.Error())
		return
	}
	state.Live = types.BoolValue(isLive)

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *workspaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan workspaceResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Also retrieve current state
	var state workspaceResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Tecton does not support renaming a workspace or changing it between live/dev. So if anything is different
	// we need to fail.
	if state.Name != plan.Name {
		resp.Diagnostics.AddError(
			"Error Updating Workspace",
			fmt.Sprintf(
				"Tecton does not support renaming workspaces, so cannot rename workspace '%v' to '%v'",
				state.Name.ValueString(),
				plan.Name.ValueString(),
			),
		)
	}

	if state.Live != plan.Live {
		resp.Diagnostics.AddError(
			"Error Updating Workspace",
			fmt.Sprintf(
				"Tecton does not support updating whether a workspace is live or development, so cannot change `live` field from '%v' to '%v'",
				state.Live.ValueBool(),
				plan.Live.ValueBool(),
			),
		)
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *workspaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Get current state
	var state workspaceResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete workspace
	var cmd = exec.Command("tecton", "workspace", "delete", "--yes", state.Name.ValueString())
	cmd.Env = r.CommandEnv
	tflog.Info(ctx, fmt.Sprintf("Deleting workspace '%v'", state.Name.ValueString()))

	output, err := cmd.CombinedOutput()
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to delete Tecton workspace",
			fmt.Sprintf("Command to delete Tecton workspace '%v' failed.\nError: %v\nOutput: %v", state.Name.ValueString(), err.Error(), string(output)),
		)
		return
	}
}

func (r *workspaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Retrieve import ID and save to id attribute
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Scans prefetched workspace data for a particular workspace. Returns (isLive, error) where isLive is true
// if the workspace is a live workspace, and false if it is a development workspace. If error != nil, then
// the value of isLive is undefined.
func GetWorkspace(ctx context.Context, workspaces Workspaces, workspaceName string) (bool, error) {
	var workspaceFound = false
	var isLive = false
	for _, ws := range workspaces.Lives {
		if ws == workspaceName {
			isLive = true
			workspaceFound = true
		}
	}
	for _, ws := range workspaces.Devs {
		if ws == workspaceName {
			isLive = false
			workspaceFound = true
		}
	}
	if !workspaceFound {
		return false, errors.New(fmt.Sprintf("Tecton workspace with name '%v' does not exist.", workspaceName))
	}
	return isLive, nil
}
