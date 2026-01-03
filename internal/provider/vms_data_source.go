// Copyright (c) German Arutyunov
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gaarutyunov/terraform-provider-slicer/internal/slicer"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &VMsDataSource{}

func NewVMsDataSource() datasource.DataSource {
	return &VMsDataSource{}
}

// VMsDataSource defines the data source implementation.
type VMsDataSource struct {
	client *slicer.SlicerClient
}

// VMsDataSourceModel describes the data source data model.
type VMsDataSourceModel struct {
	Filter     types.List  `tfsdk:"filter"`
	VMs        types.List  `tfsdk:"vms"`
	TotalCount types.Int64 `tfsdk:"total_count"`
}

// VMsFilterModel describes a filter block.
type VMsFilterModel struct {
	Tag types.String `tfsdk:"tag"`
}

// VMsVMModel describes a VM in the list.
type VMsVMModel struct {
	Hostname  types.String `tfsdk:"hostname"`
	IP        types.String `tfsdk:"ip"`
	CPUs      types.Int64  `tfsdk:"cpus"`
	RamGB     types.Int64  `tfsdk:"ram_gb"`
	Arch      types.String `tfsdk:"arch"`
	Tags      types.Map    `tfsdk:"tags"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (d *VMsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vms"
}

func (d *VMsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches a list of Slicer VMs with optional filtering.",

		Attributes: map[string]schema.Attribute{
			"vms": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of VMs matching the filter.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"hostname": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The hostname of the VM.",
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
				},
			},
			"total_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The number of VMs matching the filter.",
			},
		},
		Blocks: map[string]schema.Block{
			"filter": schema.ListNestedBlock{
				MarkdownDescription: "Filter criteria for VMs.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"tag": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Filter by tag (key=value format).",
						},
					},
				},
			},
		},
	}
}

func (d *VMsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *VMsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data VMsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Parse filters
	var filters []VMsFilterModel
	if !data.Filter.IsNull() {
		resp.Diagnostics.Append(data.Filter.ElementsAs(ctx, &filters, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	tflog.Debug(ctx, "Listing VMs", map[string]interface{}{
		"filter_count": len(filters),
	})

	// List all VMs
	vms, err := d.client.ListVMs(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list VMs: %s", err))
		return
	}

	// Apply filters
	var filteredVMs []slicer.SlicerNode
	for _, vm := range vms {
		if matchesFilters(vm, filters) {
			filteredVMs = append(filteredVMs, vm)
		}
	}

	// Convert to model
	vmModels := make([]VMsVMModel, 0, len(filteredVMs))
	for _, vm := range filteredVMs {
		// Parse IP (remove CIDR notation if present)
		ip := vm.IP
		if strings.Contains(ip, "/") {
			ip = strings.Split(ip, "/")[0]
		}

		vmModel := VMsVMModel{
			Hostname:  types.StringValue(vm.Hostname),
			IP:        types.StringValue(ip),
			Arch:      types.StringValue(vm.Arch),
			CreatedAt: types.StringValue(vm.CreatedAt.Format(time.RFC3339)),
		}

		if vm.CPUs > 0 {
			vmModel.CPUs = types.Int64Value(int64(vm.CPUs))
		} else {
			vmModel.CPUs = types.Int64Null()
		}

		if vm.RamBytes > 0 {
			vmModel.RamGB = types.Int64Value(vm.RamBytes / (1024 * 1024 * 1024))
		} else {
			vmModel.RamGB = types.Int64Null()
		}

		// Parse tags
		if len(vm.Tags) > 0 {
			tags := make(map[string]string)
			for _, tag := range vm.Tags {
				parts := strings.SplitN(tag, "=", 2)
				if len(parts) == 2 {
					tags[parts[0]] = parts[1]
				}
			}
			tagsValue, diags := types.MapValueFrom(ctx, types.StringType, tags)
			resp.Diagnostics.Append(diags...)
			if !resp.Diagnostics.HasError() {
				vmModel.Tags = tagsValue
			}
		} else {
			vmModel.Tags = types.MapNull(types.StringType)
		}

		vmModels = append(vmModels, vmModel)
	}

	vmsValue, diags := types.ListValueFrom(ctx, types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"hostname":   types.StringType,
			"ip":         types.StringType,
			"cpus":       types.Int64Type,
			"ram_gb":     types.Int64Type,
			"arch":       types.StringType,
			"tags":       types.MapType{ElemType: types.StringType},
			"created_at": types.StringType,
		},
	}, vmModels)
	resp.Diagnostics.Append(diags...)

	data.VMs = vmsValue
	data.TotalCount = types.Int64Value(int64(len(filteredVMs)))

	tflog.Trace(ctx, "Listed VMs", map[string]interface{}{
		"count": len(filteredVMs),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func matchesFilters(vm slicer.SlicerNode, filters []VMsFilterModel) bool {
	if len(filters) == 0 {
		return true
	}

	for _, filter := range filters {
		if !filter.Tag.IsNull() {
			tagFilter := filter.Tag.ValueString()
			found := false
			for _, tag := range vm.Tags {
				if tag == tagFilter || strings.Contains(tag, tagFilter) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}
