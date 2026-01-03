// Copyright (c) German Arutyunov
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/gaarutyunov/terraform-provider-slicer/internal/slicer"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &SecretDataSource{}

func NewSecretDataSource() datasource.DataSource {
	return &SecretDataSource{}
}

// SecretDataSource defines the data source implementation.
type SecretDataSource struct {
	client *slicer.SlicerClient
}

// SecretDataSourceModel describes the data source data model.
type SecretDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	Size        types.Int64  `tfsdk:"size"`
	Permissions types.String `tfsdk:"permissions"`
	UID         types.Int64  `tfsdk:"uid"`
	GID         types.Int64  `tfsdk:"gid"`
}

func (d *SecretDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (d *SecretDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches metadata about a Slicer secret. Note: The actual secret value is not returned for security reasons.",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the secret to look up.",
			},
			"size": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The size of the secret data in bytes.",
			},
			"permissions": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "File permissions of the secret.",
			},
			"uid": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Owner UID of the secret file.",
			},
			"gid": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Group GID of the secret file.",
			},
		},
	}
}

func (d *SecretDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SecretDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SecretDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading secret", map[string]interface{}{
		"name": data.Name.ValueString(),
	})

	// List secrets and find the one we're looking for
	secrets, err := d.client.ListSecrets(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list secrets: %s", err))
		return
	}

	var found *slicer.Secret
	for _, secret := range secrets {
		if secret.Name == data.Name.ValueString() {
			found = &secret
			break
		}
	}

	if found == nil {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Secret with name '%s' not found", data.Name.ValueString()))
		return
	}

	data.Size = types.Int64Value(found.Size)
	data.Permissions = types.StringValue(found.Permissions)
	data.UID = types.Int64Value(int64(found.UID))
	data.GID = types.Int64Value(int64(found.GID))

	tflog.Trace(ctx, "Read secret", map[string]interface{}{
		"name": data.Name.ValueString(),
		"size": found.Size,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
