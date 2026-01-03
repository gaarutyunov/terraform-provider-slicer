// Copyright (c) German Arutyunov
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/gaarutyunov/terraform-provider-slicer/internal/slicer"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ExecResource{}

func NewExecResource() resource.Resource {
	return &ExecResource{}
}

// ExecResource defines the resource implementation.
type ExecResource struct {
	client *slicer.SlicerClient
}

// ExecResourceModel describes the resource data model.
type ExecResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Hostname types.String `tfsdk:"hostname"`
	Command  types.String `tfsdk:"command"`
	Args     types.List   `tfsdk:"args"`
	User     types.String `tfsdk:"user"`
	UID      types.Int64  `tfsdk:"uid"`
	GID      types.Int64  `tfsdk:"gid"`
	Workdir  types.String `tfsdk:"workdir"`
	Shell    types.String `tfsdk:"shell"`
	Triggers types.Map    `tfsdk:"triggers"`
	ExitCode types.Int64  `tfsdk:"exit_code"`
	Stdout   types.String `tfsdk:"stdout"`
	Stderr   types.String `tfsdk:"stderr"`
}

func (r *ExecResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_exec"
}

func (r *ExecResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Executes a command on a Slicer VM. The command runs on create and when triggers change.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the exec resource.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"hostname": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The hostname of the VM to execute the command on.",
			},
			"command": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The command to execute.",
			},
			"args": schema.ListAttribute{
				Optional:            true,
				MarkdownDescription: "Arguments to pass to the command.",
				ElementType:         types.StringType,
			},
			"user": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "User to run the command as (deprecated, use uid instead).",
				Default:             stringdefault.StaticString("root"),
			},
			"uid": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "User ID to run the command as. Defaults to 0 (root).",
				Default:             int64default.StaticInt64(0),
			},
			"gid": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Group ID to run the command as. Defaults to 0 (root).",
				Default:             int64default.StaticInt64(0),
			},
			"workdir": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Working directory for the command.",
			},
			"shell": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Shell to use for command execution (e.g., '/bin/bash').",
			},
			"triggers": schema.MapAttribute{
				Optional:            true,
				MarkdownDescription: "A map of values that, when changed, will cause the command to re-run.",
				ElementType:         types.StringType,
			},
			"exit_code": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The exit code of the command.",
			},
			"stdout": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The standard output of the command.",
			},
			"stderr": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The standard error of the command.",
			},
		},
	}
}

func (r *ExecResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*SlicerProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *SlicerProviderData, got: %T", req.ProviderData),
		)
		return
	}

	r.client = providerData.Client
}

func (r *ExecResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ExecResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Execute the command
	stdout, stderr, exitCode, err := r.executeCommand(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Execution Error", fmt.Sprintf("Unable to execute command: %s", err))
		return
	}

	// Set computed values
	data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.Hostname.ValueString(), data.Command.ValueString()))
	data.ExitCode = types.Int64Value(int64(exitCode))
	data.Stdout = types.StringValue(stdout)
	data.Stderr = types.StringValue(stderr)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ExecResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ExecResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Exec resources are not readable - they represent a one-time execution
	// Just keep the existing state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ExecResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ExecResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Re-execute the command when triggers change
	stdout, stderr, exitCode, err := r.executeCommand(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Execution Error", fmt.Sprintf("Unable to execute command: %s", err))
		return
	}

	data.ExitCode = types.Int64Value(int64(exitCode))
	data.Stdout = types.StringValue(stdout)
	data.Stderr = types.StringValue(stderr)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ExecResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Nothing to delete - exec is a one-time operation
}

func (r *ExecResource) executeCommand(ctx context.Context, data *ExecResourceModel) (stdout, stderr string, exitCode int, err error) {
	execReq := slicer.SlicerExecRequest{
		Command: data.Command.ValueString(),
		UID:     uint32(data.UID.ValueInt64()),
		GID:     uint32(data.GID.ValueInt64()),
		Stdout:  true,
		Stderr:  true,
	}

	if !data.Args.IsNull() {
		var args []string
		data.Args.ElementsAs(ctx, &args, false)
		execReq.Args = args
	}

	if !data.Workdir.IsNull() {
		execReq.Cwd = data.Workdir.ValueString()
	}

	if !data.Shell.IsNull() {
		execReq.Shell = data.Shell.ValueString()
	}

	tflog.Debug(ctx, "Executing command", map[string]interface{}{
		"hostname": data.Hostname.ValueString(),
		"command":  data.Command.ValueString(),
	})

	resultChan, err := r.client.Exec(ctx, data.Hostname.ValueString(), execReq)
	if err != nil {
		return "", "", -1, err
	}

	var stdoutBuilder, stderrBuilder strings.Builder

	for result := range resultChan {
		if result.Error != "" {
			return stdoutBuilder.String(), stderrBuilder.String(), result.ExitCode, fmt.Errorf("exec error: %s", result.Error)
		}
		if result.Stdout != "" {
			stdoutBuilder.WriteString(result.Stdout)
		}
		if result.Stderr != "" {
			stderrBuilder.WriteString(result.Stderr)
		}
		exitCode = result.ExitCode
	}

	tflog.Trace(ctx, "Command executed", map[string]interface{}{
		"hostname":  data.Hostname.ValueString(),
		"exit_code": exitCode,
	})

	return stdoutBuilder.String(), stderrBuilder.String(), exitCode, nil
}
