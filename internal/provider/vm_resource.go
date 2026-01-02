// Copyright (c) German Arutyunov
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/gaarutyunov/terraform-provider-slicer/internal/slicer"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &VMResource{}
var _ resource.ResourceWithImportState = &VMResource{}

func NewVMResource() resource.Resource {
	return &VMResource{}
}

// VMResource defines the resource implementation.
type VMResource struct {
	client *slicer.SlicerClient
}

// VMResourceModel describes the resource data model.
type VMResourceModel struct {
	ID         types.String `tfsdk:"id"`
	HostGroup  types.String `tfsdk:"host_group"`
	Hostname   types.String `tfsdk:"hostname"`
	IP         types.String `tfsdk:"ip"`
	CPUs       types.Int64  `tfsdk:"cpus"`
	RamGB      types.Int64  `tfsdk:"ram_gb"`
	Persistent types.Bool   `tfsdk:"persistent"`
	DiskImage  types.String `tfsdk:"disk_image"`
	ImportUser types.String `tfsdk:"import_user"`
	SSHKeys    types.List   `tfsdk:"ssh_keys"`
	Userdata   types.String `tfsdk:"userdata"`
	Tags       types.Map    `tfsdk:"tags"`
	Secrets    types.List   `tfsdk:"secrets"`
	Arch       types.String `tfsdk:"arch"`
	CreatedAt  types.String `tfsdk:"created_at"`
}

func (r *VMResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

func (r *VMResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Slicer VM.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the VM (hostname).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"host_group": schema.StringAttribute{
				MarkdownDescription: "The host group to create the VM in (e.g., 'w1-medium').",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hostname": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The auto-generated hostname of the VM.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ip": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The IP address of the VM.",
			},
			"cpus": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Number of CPUs. Defaults to host group setting.",
				Default:             int64default.StaticInt64(0),
			},
			"ram_gb": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "RAM in GB. Defaults to host group setting.",
				Default:             int64default.StaticInt64(0),
			},
			"persistent": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Enable persistent storage.",
				Default:             booldefault.StaticBool(false),
			},
			"disk_image": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Custom disk image to use.",
			},
			"import_user": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Import SSH keys from GitHub user.",
			},
			"ssh_keys": schema.ListAttribute{
				Optional:            true,
				MarkdownDescription: "List of SSH public keys to inject.",
				ElementType:         types.StringType,
			},
			"userdata": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Cloud-init userdata script.",
			},
			"tags": schema.MapAttribute{
				Optional:            true,
				MarkdownDescription: "Tags to apply to the VM (key=value format).",
				ElementType:         types.StringType,
			},
			"secrets": schema.ListAttribute{
				Optional:            true,
				MarkdownDescription: "List of secret names to inject into the VM.",
				ElementType:         types.StringType,
			},
			"arch": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The architecture of the VM (e.g., 'amd64').",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The creation timestamp of the VM.",
			},
		},
	}
}

func (r *VMResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *VMResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data VMResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build create request
	createReq := slicer.SlicerCreateNodeRequest{
		Persistent: data.Persistent.ValueBool(),
	}

	if !data.CPUs.IsNull() && data.CPUs.ValueInt64() > 0 {
		createReq.CPUs = int(data.CPUs.ValueInt64())
	}

	if !data.RamGB.IsNull() && data.RamGB.ValueInt64() > 0 {
		createReq.RamBytes = slicer.GiB(data.RamGB.ValueInt64())
	}

	if !data.DiskImage.IsNull() {
		createReq.DiskImage = data.DiskImage.ValueString()
	}

	if !data.ImportUser.IsNull() {
		createReq.ImportUser = data.ImportUser.ValueString()
	}

	if !data.SSHKeys.IsNull() {
		var sshKeys []string
		resp.Diagnostics.Append(data.SSHKeys.ElementsAs(ctx, &sshKeys, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createReq.SSHKeys = sshKeys
	}

	if !data.Userdata.IsNull() {
		createReq.Userdata = data.Userdata.ValueString()
	}

	if !data.Tags.IsNull() {
		var tags map[string]string
		resp.Diagnostics.Append(data.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for k, v := range tags {
			createReq.Tags = append(createReq.Tags, fmt.Sprintf("%s=%s", k, v))
		}
	}

	if !data.Secrets.IsNull() {
		var secrets []string
		resp.Diagnostics.Append(data.Secrets.ElementsAs(ctx, &secrets, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createReq.Secrets = secrets
	}

	tflog.Debug(ctx, "Creating VM", map[string]interface{}{
		"host_group": data.HostGroup.ValueString(),
	})

	// Create the VM
	result, err := r.client.CreateVM(ctx, data.HostGroup.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create VM: %s", err))
		return
	}

	// Parse IP (remove CIDR notation if present)
	ip := result.IP
	if strings.Contains(ip, "/") {
		ip = strings.Split(ip, "/")[0]
	}

	// Set computed values
	data.ID = types.StringValue(result.Hostname)
	data.Hostname = types.StringValue(result.Hostname)
	data.IP = types.StringValue(ip)
	data.Arch = types.StringValue(result.Arch)
	data.CreatedAt = types.StringValue(result.CreatedAt.Format(time.RFC3339))

	tflog.Trace(ctx, "Created VM", map[string]interface{}{
		"hostname": result.Hostname,
		"ip":       ip,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VMResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data VMResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// List all VMs and find ours
	vms, err := r.client.ListVMs(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list VMs: %s", err))
		return
	}

	var found *slicer.SlicerNode
	for _, vm := range vms {
		if vm.Hostname == data.Hostname.ValueString() {
			found = &vm
			break
		}
	}

	if found == nil {
		// VM was deleted outside of Terraform
		resp.State.RemoveResource(ctx)
		return
	}

	// Parse IP (remove CIDR notation if present)
	ip := found.IP
	if strings.Contains(ip, "/") {
		ip = strings.Split(ip, "/")[0]
	}

	// Update state with current values
	data.IP = types.StringValue(ip)
	data.Arch = types.StringValue(found.Arch)
	data.CreatedAt = types.StringValue(found.CreatedAt.Format(time.RFC3339))

	if found.CPUs > 0 {
		data.CPUs = types.Int64Value(int64(found.CPUs))
	}
	if found.RamBytes > 0 {
		data.RamGB = types.Int64Value(found.RamBytes / (1024 * 1024 * 1024))
	}

	// Parse tags
	if len(found.Tags) > 0 {
		tags := make(map[string]string)
		for _, tag := range found.Tags {
			parts := strings.SplitN(tag, "=", 2)
			if len(parts) == 2 {
				tags[parts[0]] = parts[1]
			}
		}
		tagsValue, diags := types.MapValueFrom(ctx, types.StringType, tags)
		resp.Diagnostics.Append(diags...)
		if !resp.Diagnostics.HasError() {
			data.Tags = tagsValue
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VMResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data VMResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Slicer doesn't support updating VMs in place
	// Most changes require replacement (handled by RequiresReplace plan modifier)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VMResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data VMResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting VM", map[string]interface{}{
		"hostname":   data.Hostname.ValueString(),
		"host_group": data.HostGroup.ValueString(),
	})

	_, err := r.client.DeleteVM(ctx, data.HostGroup.ValueString(), data.Hostname.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete VM: %s", err))
		return
	}

	tflog.Trace(ctx, "Deleted VM", map[string]interface{}{
		"hostname": data.Hostname.ValueString(),
	})
}

func (r *VMResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: host_group/hostname
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			"Import ID must be in the format: host_group/hostname",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("host_group"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("hostname"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
