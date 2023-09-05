// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure ScaffoldingProvider satisfies various provider interfaces.
var _ provider.Provider = &TectonProvider{}

// New is a helper function to simplify provider server and testing implementation.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &TectonProvider{
			version: version,
		}
	}
}

// TectonProvider defines the provider implementation.
type TectonProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// TectonProviderModel maps provider schema data to a Go type.
type TectonProviderModel struct {
	Url    types.String `tfsdk:"url"`
	ApiKey types.String `tfsdk:"api_key"`
}

// Workspaces stores all the workspaces we've found on the Tecton instance.
type Workspaces struct {
	Lives []string
	Devs  []string
}

// ProviderData stores all the data that datasources and resources need from
// the provider.
type ProviderData struct {
	CommandEnv    []string
	WorkspaceData Workspaces
}

// Metadata returns the provider type name.
func (p *TectonProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "tecton"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *TectonProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Required: true,
			},
			"api_key": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
			},
		},
	}
}

// Configure prepares a Tecton API client for data sources and resources.
func (p *TectonProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// Ensure Tecton CLI is installed
	_, err := exec.LookPath("tecton")
	if err != nil {
		resp.Diagnostics.AddError(
			"Tecton CLI not installed",
			"Didn't find 'tecton' executable, which is required to run this provider. Please install it via `pip install tecton`")
		return
	}

	// Retrieve provider data from configuration
	var config TectonProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// All Tecton commands for this provider must be issued with these envvars to
	//		(1) Point to the correct Tecton instance
	//  	(2) Properly authenticate with the Tecton instance
	commandEnv := append(
		os.Environ(),
		fmt.Sprintf("TECTON_API_KEY=%v", config.ApiKey.ValueString()),
		fmt.Sprintf("API_SERVICE=%v/api", config.Url.ValueString()),
	)

	// Pre-fetch all the workspaces since they can only be fetched all at once
	// and since each call takes a few seconds. This data should only be
	// used during `terraform plan` (e.g. the `Read` function) and not
	// `terraform apply` since deletions and creations will make this
	// data stale.
	tflog.Info(ctx, "Pre-fetching workspace list")
	workspaces, err := ListWorkspaces(ctx, commandEnv)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to list Tecton workspaces",
			fmt.Sprintf(
				"Command to list Tecton workspaces failed.\nError: %v",
				err,
			),
		)
		return
	}

	providerData := ProviderData{
		commandEnv,
		workspaces,
	}
	resp.DataSourceData = providerData
	resp.ResourceData = providerData

	tflog.Info(ctx, "Configured Tecton provider")
}

// DataSources defines the data sources implemented in the provider.
func (p *TectonProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewWorkspaceResource,
	}
}

// Resources defines the resources implemented in the provider.
func (p *TectonProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}

// Query the complete list of workspaces in the Tecton instance. And parse the output
// An example output from `tecton workspace list` is the following:
// ```
// Live Workspaces:
//   a
//   b
//
// Development Workspaces:
//   c
// * d
//   e
// ```
// Where the '*' character begins the line of the current "active" workspace. The concept of an
// active workspace is not used in this provider, but we still need to handle it in this parsing
// function.
//
// The expected output of this function given the above ouput from Tecton is the following
// ```
// Workspace{
//    Lives: []string{"a", "b"}
//    Devs:  []string{"c", "d", "e"}
// }
// ```
func ListWorkspaces(ctx context.Context, commandEnv []string) (Workspaces, error) {
	cmd := exec.Command("tecton", "workspace", "list")
	cmd.Env = commandEnv
	output, err := cmd.CombinedOutput()
	if err != nil {
		err := errors.New(fmt.Sprintf("%v\nOutput: %v", err.Error(), string(output)))
		return Workspaces{}, err
	}

	// Assert the output matches the expected regex
	expectedOutputRegex := regexp.MustCompile("Live Workspaces:\\n(\\*? +([^ ]+)\\n?)*\\nDevelopment Workspaces:\\n(\\*? +([^ ]+)\\n?)*")
	matches := expectedOutputRegex.Match(output)
	if !matches {
		err := errors.New(fmt.Sprintf(
			"`tecton workspace list` returned unexpected output.\nExpected to match regex: %v\nGot:\"%v\"",
			expectedOutputRegex,
			string(output),
		))
		return Workspaces{}, err
	}

	lines := strings.Split(string(output), "\n")

	workspaces := Workspaces{}

	// Iterate over the lines and populate the `lives` and `devs` fields of the `Workspaces` object.
	var liveSection = true
	for _, line := range lines {
		if strings.HasPrefix(line, "Live Workspaces:") {
			liveSection = true
			continue
		}

		if strings.HasPrefix(line, "Development Workspaces:") {
			liveSection = false
			continue
		}

		// One workspace line will start with "*"
		workspace := strings.TrimPrefix(line, "*")
		workspace = strings.TrimSpace(workspace)

		if workspace == "" {
			continue
		}

		// Add the workspace name to the appropriate field of the `Workspaces` object.
		if liveSection {
			workspaces.Lives = append(workspaces.Lives, workspace)
		} else {
			workspaces.Devs = append(workspaces.Devs, workspace)
		}
	}
	return workspaces, nil
}
