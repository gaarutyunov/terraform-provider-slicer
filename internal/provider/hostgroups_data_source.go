// Copyright (c) German Arutyunov
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/gaarutyunov/terraform-provider-slicer/internal/slicer"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &HostgroupsDataSource{}

func NewHostgroupsDataSource() datasource.DataSource {
	return &HostgroupsDataSource{}
}

// HostgroupsDataSource defines the data source implementation.
type HostgroupsDataSource struct {
	client *slicer.SlicerClient
}

// HostgroupsDataSourceModel describes the data source data model.
type HostgroupsDataSourceModel struct {
	Names      types.List `tfsdk:"names"`
	Hostgroups types.List `tfsdk:"hostgroups"`
}

// HostgroupModel describes a hostgroup in the list.
type HostgroupModel struct {
	Name     types.String `tfsdk:"name"`
	Count    types.Int64  `tfsdk:"count"`
	CPUs     types.Int64  `tfsdk:"cpus"`
	RamGB    types.Int64  `tfsdk:"ram_gb"`
	Arch     types.String `tfsdk:"arch"`
	GPUCount types.Int64  `tfsdk:"gpu_count"`
}

func (d *HostgroupsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hostgroups"
}

func (d *HostgroupsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches available Slicer host groups.",

		Attributes: map[string]schema.Attribute{
			"names": schema.ListAttribute{
				Computed:            true,
				MarkdownDescription: "List of host group names.",
				ElementType:         types.StringType,
			},
			"hostgroups": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Detailed list of host groups.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The name of the host group.",
						},
						"count": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Number of VMs in the host group.",
						},
						"cpus": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Number of CPUs per VM.",
						},
						"ram_gb": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "RAM per VM in GB.",
						},
						"arch": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Architecture of the host group.",
						},
						"gpu_count": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Number of GPUs per VM.",
						},
					},
				},
			},
		},
	}
}

func (d *HostgroupsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *HostgroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data HostgroupsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Listing host groups")

	hostgroups, err := d.client.GetHostGroups(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list host groups: %s", err))
		return
	}

	// Build names list
	names := make([]string, 0, len(hostgroups))
	for _, hg := range hostgroups {
		names = append(names, hg.Name)
	}

	namesValue, diags := types.ListValueFrom(ctx, types.StringType, names)
	resp.Diagnostics.Append(diags...)
	data.Names = namesValue

	// Build detailed list
	hgModels := make([]HostgroupModel, 0, len(hostgroups))
	for _, hg := range hostgroups {
		hgModel := HostgroupModel{
			Name:     types.StringValue(hg.Name),
			Count:    types.Int64Value(int64(hg.Count)),
			CPUs:     types.Int64Value(int64(hg.CPUs)),
			RamGB:    types.Int64Value(hg.RamBytes / (1024 * 1024 * 1024)),
			Arch:     types.StringValue(hg.Arch),
			GPUCount: types.Int64Value(int64(hg.GPUCount)),
		}
		hgModels = append(hgModels, hgModel)
	}

	hgValue, diags := types.ListValueFrom(ctx, types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"name":      types.StringType,
			"count":     types.Int64Type,
			"cpus":      types.Int64Type,
			"ram_gb":    types.Int64Type,
			"arch":      types.StringType,
			"gpu_count": types.Int64Type,
		},
	}, hgModels)
	resp.Diagnostics.Append(diags...)
	data.Hostgroups = hgValue

	tflog.Trace(ctx, "Listed host groups", map[string]interface{}{
		"count": len(hostgroups),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
