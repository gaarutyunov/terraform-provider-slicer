// Copyright (c) German Arutyunov
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/gaarutyunov/terraform-provider-slicer/internal/slicer"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &VMDataSource{}

func NewVMDataSource() datasource.DataSource {
	return &VMDataSource{}
}

// VMDataSource defines the data source implementation.
type VMDataSource struct {
	client *slicer.SlicerClient
}

// VMDataSourceModel describes the data source data model.
type VMDataSourceModel struct {
	Hostname  types.String `tfsdk:"hostname"`
	IP        types.String `tfsdk:"ip"`
	CPUs      types.Int64  `tfsdk:"cpus"`
	RamGB     types.Int64  `tfsdk:"ram_gb"`
	Arch      types.String `tfsdk:"arch"`
	Tags      types.Map    `tfsdk:"tags"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (d *VMDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

func (d *VMDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches information about an existing Slicer VM.",

		Attributes: map[string]schema.Attribute{
			"hostname": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The hostname of the VM to look up.",
			},
			"ip": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The IP address of the VM.",
			},
			"cpus": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of CPUs.",
			},
			"ram_gb": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "RAM in GB.",
			},
			"arch": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The architecture of the VM.",
			},
			"tags": schema.MapAttribute{
				Computed:            true,
				MarkdownDescription: "Tags applied to the VM.",
				ElementType:         types.StringType,
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The creation timestamp of the VM.",
			},
		},
	}
}

func (d *VMDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*SlicerProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *SlicerProviderData, got: %T", req.ProviderData),
		)
		return
	}

	d.client = providerData.Client
}

func (d *VMDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data VMDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading VM", map[string]interface{}{
		"hostname": data.Hostname.ValueString(),
	})

	// List all VMs and find the one we're looking for
	vms, err := d.client.ListVMs(ctx)
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
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("VM with hostname '%s' not found", data.Hostname.ValueString()))
		return
	}

	// Parse IP (remove CIDR notation if present)
	ip := found.IP
	if strings.Contains(ip, "/") {
		ip = strings.Split(ip, "/")[0]
	}

	data.IP = types.StringValue(ip)
	data.Arch = types.StringValue(found.Arch)
	data.CreatedAt = types.StringValue(found.CreatedAt.Format(time.RFC3339))

	if found.CPUs > 0 {
		data.CPUs = types.Int64Value(int64(found.CPUs))
	} else {
		data.CPUs = types.Int64Null()
	}

	if found.RamBytes > 0 {
		data.RamGB = types.Int64Value(found.RamBytes / (1024 * 1024 * 1024))
	} else {
		data.RamGB = types.Int64Null()
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
	} else {
		data.Tags = types.MapNull(types.StringType)
	}

	tflog.Trace(ctx, "Read VM", map[string]interface{}{
		"hostname": data.Hostname.ValueString(),
		"ip":       ip,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
