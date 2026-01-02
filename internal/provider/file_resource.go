// Copyright (c) German Arutyunov
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

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
var _ resource.Resource = &FileResource{}

func NewFileResource() resource.Resource {
	return &FileResource{}
}

// FileResource defines the resource implementation.
type FileResource struct {
	client *slicer.SlicerClient
}

// FileResourceModel describes the resource data model.
type FileResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Hostname    types.String `tfsdk:"hostname"`
	Destination types.String `tfsdk:"destination"`
	Content     types.String `tfsdk:"content"`
	Source      types.String `tfsdk:"source"`
	Permissions types.String `tfsdk:"permissions"`
	Owner       types.Int64  `tfsdk:"owner"`
	Group       types.Int64  `tfsdk:"group"`
	ContentHash types.String `tfsdk:"content_hash"`
}

func (r *FileResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_file"
}

func (r *FileResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Copies a file to a Slicer VM.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the file resource.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"hostname": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The hostname of the VM to copy the file to.",
			},
			"destination": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The destination path on the VM.",
			},
			"content": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The content of the file. Conflicts with `source`.",
				Sensitive:           true,
			},
			"source": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The local source file path. Conflicts with `content`.",
			},
			"permissions": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "File permissions (e.g., '0644').",
				Default:             stringdefault.StaticString("0644"),
			},
			"owner": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Owner UID. Defaults to 0 (root).",
				Default:             int64default.StaticInt64(0),
			},
			"group": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Group GID. Defaults to 0 (root).",
				Default:             int64default.StaticInt64(0),
			},
			"content_hash": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SHA256 hash of the file content.",
			},
		},
	}
}

func (r *FileResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *FileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data FileResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate that either content or source is specified
	if data.Content.IsNull() && data.Source.IsNull() {
		resp.Diagnostics.AddError(
			"Missing File Content",
			"Either 'content' or 'source' must be specified.",
		)
		return
	}

	if !data.Content.IsNull() && !data.Source.IsNull() {
		resp.Diagnostics.AddError(
			"Conflicting Attributes",
			"Only one of 'content' or 'source' can be specified.",
		)
		return
	}

	// Copy file to VM
	contentHash, err := r.copyFile(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Copy Error", fmt.Sprintf("Unable to copy file: %s", err))
		return
	}

	// Set computed values
	data.ID = types.StringValue(fmt.Sprintf("%s:%s", data.Hostname.ValueString(), data.Destination.ValueString()))
	data.ContentHash = types.StringValue(contentHash)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data FileResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// File resources are not fully readable from the VM
	// We keep the existing state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data FileResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Re-copy the file
	contentHash, err := r.copyFile(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Copy Error", fmt.Sprintf("Unable to copy file: %s", err))
		return
	}

	data.ContentHash = types.StringValue(contentHash)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data FileResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete the file from VM by executing rm command
	execReq := slicer.SlicerExecRequest{
		Command: "rm",
		Args:    []string{"-f", data.Destination.ValueString()},
		UID:     0,
		GID:     0,
	}

	resultChan, err := r.client.Exec(ctx, data.Hostname.ValueString(), execReq)
	if err != nil {
		resp.Diagnostics.AddWarning("Delete Warning", fmt.Sprintf("Unable to delete file: %s", err))
		return
	}

	// Drain the channel
	for range resultChan {
	}

	tflog.Trace(ctx, "Deleted file", map[string]interface{}{
		"hostname":    data.Hostname.ValueString(),
		"destination": data.Destination.ValueString(),
	})
}

func (r *FileResource) copyFile(ctx context.Context, data *FileResourceModel) (string, error) {
	var content []byte
	var err error

	if !data.Content.IsNull() {
		content = []byte(data.Content.ValueString())
	} else {
		content, err = os.ReadFile(data.Source.ValueString())
		if err != nil {
			return "", fmt.Errorf("failed to read source file: %w", err)
		}
	}

	// Calculate content hash
	hash := sha256.Sum256(content)
	contentHash := fmt.Sprintf("%x", hash)

	// Write content to temp file
	tmpFile, err := os.CreateTemp("", "slicer-file-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	tflog.Debug(ctx, "Copying file to VM", map[string]interface{}{
		"hostname":    data.Hostname.ValueString(),
		"destination": data.Destination.ValueString(),
		"size":        len(content),
	})

	// Copy file to VM using binary mode
	err = r.client.CpToVM(
		ctx,
		data.Hostname.ValueString(),
		tmpFile.Name(),
		data.Destination.ValueString(),
		uint32(data.Owner.ValueInt64()),
		uint32(data.Group.ValueInt64()),
		data.Permissions.ValueString(),
		"binary",
	)
	if err != nil {
		return "", fmt.Errorf("failed to copy file to VM: %w", err)
	}

	tflog.Trace(ctx, "Copied file to VM", map[string]interface{}{
		"hostname":     data.Hostname.ValueString(),
		"destination":  data.Destination.ValueString(),
		"content_hash": contentHash,
	})

	return contentHash, nil
}

// getAbsPath returns the absolute path, handling relative paths
func getAbsPath(p string) (string, error) {
	if filepath.IsAbs(p) {
		return p, nil
	}
	return filepath.Abs(p)
}
