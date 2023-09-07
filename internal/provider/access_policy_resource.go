package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/exp/slices"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
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
	_ resource.Resource                = &accessPolicyResource{}
	_ resource.ResourceWithConfigure   = &accessPolicyResource{}
	_ resource.ResourceWithImportState = &accessPolicyResource{}
)

// NewWorkspaceResource is a helper function to simplify the provider implementation.
func NewAccessPolicyResource() resource.Resource {
	return &accessPolicyResource{}
}

// accessPolicyResource is the resource implementation.
type accessPolicyResource struct {
	CommandEnv []string
}

// The valid roles, in order of increasing power
var validRoles = []string{"viewer", "operator", "editor", "owner"}

// accessPolicyResourceModel maps the resource schema data.
type accessPolicyResourceModel struct {
	ID               types.String              `tfsdk:"id"`
	LastUpdated      types.String              `tfsdk:"last_updated"`
	UserID           types.String              `tfsdk:"user_id"`
	ServiceAccountID types.String              `tfsdk:"service_account_id"`
	Admin            types.Bool                `tfsdk:"admin"`
	AllWorkspaces    []types.String            `tfsdk:"all_workspaces"`
	Workspaces       map[string][]types.String `tfsdk:"workspaces"`
}

// A policy for a single workspace (or organization) in the JSON output of `tecton access-control get-roles`
type tectonGetRolesPolicy struct {
	ResourceType  string                      `json:"resource_type"`
	WorkspaceName string                      `json:"workspace_name,omitempty"`
	RolesGranted  []tectonGetRolesRoleGranted `json:"roles_granted"`
}

// A single role (e.g. "owner") in the JSON output of `tecton access-control get-roles`
type tectonGetRolesRoleGranted struct {
	Role              string                          `json:"role"`
	AssignmentSources []tectonGetRoleAssignmentSource `json:"assignment_sources"`
}

// An assignment source (e.g. DIRECT) in the JSON output of `tecton access-control get-roles`
type tectonGetRoleAssignmentSource struct {
	AssignmentType string `json:"assignment_type"`
}

// A type to store a key-value pair in a map.
type KeyValuePair struct {
	Key   string
	Value string
}

// Configure adds the provider configured client to the resource.
func (r *accessPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
}

// Metadata returns the resource type name.
func (r *accessPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_policy"
}

// Schema defines the schema for the resource.
func (r *accessPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"last_updated": schema.StringAttribute{
				Computed: true,
			},
			"user_id": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z0-9-_.@]+$`),
						"must contain only alphanumeric characters, or characters in the set -_.@",
					),
				},
			},
			"service_account_id": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z0-9]+$`),
						"must contain only alphanumeric characters",
					),
				},
			},
			"admin": schema.BoolAttribute{
				Optional: true,
			},
			"all_workspaces": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.List{
					listvalidator.ValueStringsAre(
						stringvalidator.OneOf(validRoles...),
					),
					listvalidator.UniqueValues(),
				},
			},
			"workspaces": schema.MapAttribute{
				Optional: true,
				ElementType: types.ListType{
					ElemType: types.StringType,
				},
				Validators: []validator.Map{
					mapvalidator.ValueListsAre(
						listvalidator.ValueStringsAre(stringvalidator.OneOf(validRoles...)),
						listvalidator.UniqueValues(),
					),
				},
			},
		},
	}
}

func (r *accessPolicyResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("user_id"),
			path.MatchRoot("service_account_id"),
		),
		resourcevalidator.AtLeastOneOf(
			path.MatchRoot("admin"),
			path.MatchRoot("all_workspaces"),
			path.MatchRoot("workspaces"),
		),
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *accessPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan accessPolicyResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var entity string
	if plan.UserID.ValueString() != "" {
		entity = fmt.Sprintf("user '%v'", plan.UserID.ValueString())
	} else if plan.ServiceAccountID.ValueString() != "" {
		entity = fmt.Sprintf("service '%v'", plan.ServiceAccountID.ValueString())
	}
	tflog.Info(ctx, fmt.Sprintf("Creating access policy for %v", entity))

	// Fail if any roles already exist. The state must first be imported.
	var state accessPolicyResourceModel
	state.UserID = plan.UserID
	state.ServiceAccountID = plan.ServiceAccountID
	tflog.Info(ctx, "Creating an access_policy")
	alreadyExists, err := r.GetFromTecton(ctx, &state)
	if err != nil {
		resp.Diagnostics.AddError("Role Read Failure", err.Error())
		return
	}
	if alreadyExists {
		resp.Diagnostics.AddError(
			"Access Policy Already Exists",
			fmt.Sprintf(
				"An access policy already exists for %v on Tecton. The state must first be imported "+
					"via `terraform import` so that no permissions are accidentally deleted.",
				entity,
			),
		)
		return
	}

	// Create resource by updating from an empty state
	var emptyState accessPolicyResourceModel
	emptyState.UserID = plan.UserID
	emptyState.ServiceAccountID = plan.ServiceAccountID
	err = r.UpdateAccessPolicy(ctx, &plan, &emptyState)
	if err != nil {
		resp.Diagnostics.AddError("Access Policy Creation Failure", err.Error())
		return
	}

	// // Generated computed values
	if plan.UserID.ValueString() != "" {
		plan.ID = types.StringValue(fmt.Sprintf("user-%v", state.UserID.ValueString()))
	} else if plan.ServiceAccountID.ValueString() != "" {
		plan.ID = types.StringValue(fmt.Sprintf("service-%v", state.ServiceAccountID.ValueString()))
	}
	plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC850)) // Time format copy-pasted from Hashicorp tutorial

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *accessPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state accessPolicyResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If we imported this access policy both IDs will be empty.
	if state.UserID.ValueString() == "" && state.ServiceAccountID.ValueString() == "" {
		if strings.HasPrefix(state.ID.ValueString(), "user-") {
			state.UserID = types.StringValue(strings.TrimPrefix(state.ID.ValueString(), "user-"))
		} else if strings.HasPrefix(state.ID.ValueString(), "service-") {
			state.ServiceAccountID = types.StringValue(strings.TrimPrefix(state.ID.ValueString(), "service-"))
		} else {
			resp.Diagnostics.AddError(
				"Invalid ID prefix",
				fmt.Sprintf("Expected either 'user-' or 'service-' as a prefix, got: %v", state.ID.ValueString()),
			)
			return
		}
	}

	// Read existing policies
	_, err := r.GetFromTecton(ctx, &state)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read Tecton roles", err.Error())
		return
	}

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *accessPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan accessPolicyResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Also retrieve current state
	var state accessPolicyResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Refresh current state. We can't trust the Terraform state because a delete on a workspace
	// may already have been applied, and that delete may have altered the existing role list.
	_, err := r.GetFromTecton(ctx, &state)
	if err != nil {
		resp.Diagnostics.AddError("Role Read Failure", err.Error())
		return
	}

	err = r.UpdateAccessPolicy(ctx, &plan, &state)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update acess policy", err.Error())
	}

	plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC850))

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource.
func (r *accessPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Get current state
	var state accessPolicyResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Refresh current state. We can't trust the Terraform state because a delete on a workspace
	// may already have been applied, and that delete may have altered the existing role list.
	_, err := r.GetFromTecton(ctx, &state)
	if err != nil {
		resp.Diagnostics.AddError("Role Read Failure", err.Error())
		return
	}

	// Delete resource by updating to an empty plan
	var emptyPlan accessPolicyResourceModel
	emptyPlan.UserID = state.UserID
	emptyPlan.ServiceAccountID = state.ServiceAccountID
	err = r.UpdateAccessPolicy(ctx, &emptyPlan, &state)
	if err != nil {
		resp.Diagnostics.AddError("Unable to delete acess policy", err.Error())
	}
}

func (r *accessPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Retrieve import ID and save to id attribute
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Like Read but does not update Terraform's state. Returns true if a policy already exists in Tecton, or False otherwise.
func (r *accessPolicyResource) GetFromTecton(ctx context.Context, state *accessPolicyResourceModel) (bool, error) {
	// Read existing policies
	var args = []string{"access-control", "get-roles", "--json-out"}
	if state.UserID.ValueString() != "" {
		args = append(args, "--user", state.UserID.ValueString())
	} else if state.ServiceAccountID.ValueString() != "" {
		args = append(args, "--service-account", state.ServiceAccountID.ValueString())
	} else {
		return false, errors.New("Cannot read from Tecton without an ID. This is a bug in the provider.")
	}
	var cmd = exec.Command("tecton", args...)
	cmd.Env = r.CommandEnv
	tflog.Info(ctx, fmt.Sprintf("Reading roles for '%v'", strings.Join(args[3:], " ")))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, errors.New(
			fmt.Sprintf(
				"Command to read Tecton roles for '%v' failed.\nError: %v\nOutput: %v",
				strings.Join(args[3:], " "),
				err.Error(),
				string(output),
			),
		)
	}

	// Parse the output
	var policies []tectonGetRolesPolicy
	err = json.Unmarshal(output, &policies)
	if err != nil {
		return false, errors.New(
			fmt.Sprintf("Failed to parse output of `tecton access-control get-roles`.\nGot: %v", output),
		)
	}

	// Clear fields
	state.Admin = types.BoolValue(false)
	state.AllWorkspaces = nil
	state.Workspaces = nil

	// Map states to objects
	for _, policy := range policies {
		for _, roleGranted := range policy.RolesGranted {
			if policy.ResourceType == "ORGANIZATION" {
				if roleGranted.Role == "admin" {
					state.Admin = types.BoolValue(true)
				} else {
					if state.AllWorkspaces == nil {
						state.AllWorkspaces = []types.String{}
					}
					state.AllWorkspaces = append(state.AllWorkspaces, types.StringValue(roleGranted.Role))
				}
			} else if policy.ResourceType == "WORKSPACE" {
				if state.Workspaces == nil {
					state.Workspaces = make(map[string][]types.String)
				}
				state.Workspaces[policy.WorkspaceName] = append(
					state.Workspaces[policy.WorkspaceName],
					types.StringValue(roleGranted.Role),
				)
			}
		}
	}

	// Sort the roles in order of increasing power
	roleToLevel := make(map[string]int)
	for i, role := range validRoles {
		level := i
		roleToLevel[role] = level
	}
	cmp := func(lhs types.String, rhs types.String) int {
		lhsLevel, lhsOk := roleToLevel[lhs.ValueString()]
		rhsLevel, rhsOk := roleToLevel[rhs.ValueString()]
		if !lhsOk || !rhsOk {
			return 0
		}
		return lhsLevel - rhsLevel
	}
	slices.SortFunc(state.AllWorkspaces, cmp)
	for _, roles := range state.Workspaces {
		slices.SortFunc(roles, cmp)
	}
	return len(policies) > 0, nil
}

// Modifies a role in Tecton for a particular user or service. If grant is true, the role will be added. If it is false, the role will be removed.
// If no workspace is provided, the role will be applied to all workspaces.
func (r *accessPolicyResource) ModifyRole(ctx context.Context, userID string, serviceAccountID string, role string, workspace string, grant bool) error {
	var accessControlSubcommand string
	if grant {
		accessControlSubcommand = "assign-role"
	} else {
		accessControlSubcommand = "unassign-role"
	}
	var args = []string{"access-control", accessControlSubcommand, "--role", role}
	if workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	if userID != "" {
		args = append(args, "--user", userID)
	} else if serviceAccountID != "" {
		args = append(args, "--service-account", serviceAccountID)
	} else {
		return errors.New("Cannot set role in Tecton without an ID. This is a bug in the provider.")
	}
	var cmd = exec.Command("tecton", args...)
	cmd.Env = r.CommandEnv
	tflog.Info(ctx, fmt.Sprintf("Running 'tecton %v'", strings.Join(args, " ")))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(
			fmt.Sprintf(
				"Command to set Tecton role failed.\nError: %v\nOutput: %v",
				err.Error(),
				string(output),
			),
		)
	}
	return nil
}

// Returns elements that are in a that are not in b
func SliceDifference(a, b []types.String) []string {
	mb := make(map[string]bool, len(b))
	for _, x := range b {
		mb[x.ValueString()] = true
	}
	var diff []string
	for _, x := range a {
		if _, found := mb[x.ValueString()]; !found {
			diff = append(diff, x.ValueString())
		}
	}
	return diff
}

// Makes the necessary calls in order to make Tecton consistent with `planRoles`
func (r *accessPolicyResource) UpdateWorkspace(
	ctx context.Context,
	userID string,
	serviceAccountID string,
	workspace string,
	planRoles []types.String,
	stateRoles []types.String,
) error {
	rolesToBeAdded := SliceDifference(planRoles, stateRoles)
	rolesToBeDeleted := SliceDifference(stateRoles, planRoles)

	// First we apply the new roles, then remove the old ones. As a requirement, at every point
	// in time during the application, the user must have either the old permission O or the new
	// permissions N. Also, after N is applied, the user should never revert back to O during
	// the application. If we revoked O before granting N, then between those two operations
	// the user would have no permissions at all, which violates our requirements. Granting N
	// before revoking O guarantees the requirements are met.
	for _, role := range rolesToBeAdded {
		err := r.ModifyRole(ctx, userID, serviceAccountID, role, workspace, true)
		if err != nil {
			return err
		}
	}
	for _, role := range rolesToBeDeleted {
		err := r.ModifyRole(ctx, userID, serviceAccountID, role, workspace, false)
		if err != nil {
			return err
		}
	}
	return nil
}

// Make the necessary calls to make Tecton consistent with this accessPolicy
func (r *accessPolicyResource) UpdateAccessPolicy(
	ctx context.Context,
	plan *accessPolicyResourceModel,
	state *accessPolicyResourceModel,
) error {
	// Handle admin
	if plan.Admin != state.Admin {
		err := r.ModifyRole(ctx, plan.UserID.ValueString(), plan.ServiceAccountID.ValueString(), "admin", "", plan.Admin.ValueBool())
		if err != nil {
			return err
		}
	}

	// Handle all_workspaces
	err := r.UpdateWorkspace(ctx, plan.UserID.ValueString(), plan.ServiceAccountID.ValueString(), "", plan.AllWorkspaces, state.AllWorkspaces)
	if err != nil {
		return err
	}

	// Handle other workspaces
	handledWorkspaces := make(map[string]bool)
	for ws, planRoles := range plan.Workspaces {
		stateRoles, _ := state.Workspaces[ws]
		err := r.UpdateWorkspace(ctx, plan.UserID.ValueString(), plan.ServiceAccountID.ValueString(), ws, planRoles, stateRoles)
		if err != nil {
			return err
		}
		handledWorkspaces[ws] = true
	}
	for ws, stateRoles := range state.Workspaces {
		if _, alreadyHandled := handledWorkspaces[ws]; alreadyHandled {
			continue
		}
		planRoles, _ := plan.Workspaces[ws]
		err := r.UpdateWorkspace(ctx, plan.UserID.ValueString(), plan.ServiceAccountID.ValueString(), ws, planRoles, stateRoles)
		if err != nil {
			return err
		}
	}
	return nil
}
