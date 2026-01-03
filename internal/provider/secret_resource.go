// Copyright (c) German Arutyunov
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/gaarutyunov/terraform-provider-slicer/internal/slicer"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &SecretResource{}
var _ resource.ResourceWithImportState = &SecretResource{}

func NewSecretResource() resource.Resource {
	return &SecretResource{}
}

// SecretResource defines the resource implementation.
type SecretResource struct {
	client *slicer.SlicerClient
}

// SecretResourceModel describes the resource data model.
type SecretResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Value       types.String `tfsdk:"value"`
	Permissions types.String `tfsdk:"permissions"`
	UID         types.Int64  `tfsdk:"uid"`
	GID         types.Int64  `tfsdk:"gid"`
}

func (r *SecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (r *SecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Slicer secret.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the secret (name).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the secret.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "The secret value.",
			},
			"permissions": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "File permissions for the secret (e.g., '0600').",
				Default:             stringdefault.StaticString("0600"),
			},
			"uid": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Owner UID for the secret file. Defaults to 0 (root).",
				Default:             int64default.StaticInt64(0),
			},
			"gid": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Group GID for the secret file. Defaults to 0 (root).",
				Default:             int64default.StaticInt64(0),
			},
		},
	}
}

func (r *SecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SecretResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := slicer.CreateSecretRequest{
		Name:        data.Name.ValueString(),
		Data:        data.Value.ValueString(),
		Permissions: data.Permissions.ValueString(),
		UID:         uint32(data.UID.ValueInt64()),
		GID:         uint32(data.GID.ValueInt64()),
	}

	tflog.Debug(ctx, "Creating secret", map[string]interface{}{
		"name": data.Name.ValueString(),
	})

	err := r.client.CreateSecret(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create secret: %s", err))
		return
	}

	data.ID = data.Name

	tflog.Trace(ctx, "Created secret", map[string]interface{}{
		"name": data.Name.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SecretResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// List secrets and check if ours exists
	secrets, err := r.client.ListSecrets(ctx)
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
		// Secret was deleted outside of Terraform
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state with current values (note: value is not returned by API)
	data.Permissions = types.StringValue(found.Permissions)
	data.UID = types.Int64Value(int64(found.UID))
	data.GID = types.Int64Value(int64(found.GID))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SecretResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := slicer.UpdateSecretRequest{
		Data:        data.Value.ValueString(),
		Permissions: data.Permissions.ValueString(),
		UID:         uint32(data.UID.ValueInt64()),
		GID:         uint32(data.GID.ValueInt64()),
	}

	tflog.Debug(ctx, "Updating secret", map[string]interface{}{
		"name": data.Name.ValueString(),
	})

	err := r.client.PatchSecret(ctx, data.Name.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update secret: %s", err))
		return
	}

	tflog.Trace(ctx, "Updated secret", map[string]interface{}{
		"name": data.Name.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SecretResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting secret", map[string]interface{}{
		"name": data.Name.ValueString(),
	})

	err := r.client.DeleteSecret(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete secret: %s", err))
		return
	}

	tflog.Trace(ctx, "Deleted secret", map[string]interface{}{
		"name": data.Name.ValueString(),
	})
}

func (r *SecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
