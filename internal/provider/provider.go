// Copyright (c) German Arutyunov
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/gaarutyunov/terraform-provider-slicer/internal/slicer"
)

// Ensure SlicerProvider satisfies various provider interfaces.
var _ provider.Provider = &SlicerProvider{}

// SlicerProvider defines the provider implementation.
type SlicerProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// SlicerProviderModel describes the provider data model.
type SlicerProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	Token    types.String `tfsdk:"token"`
	Timeout  types.String `tfsdk:"timeout"`
	Insecure types.Bool   `tfsdk:"insecure"`
}

// SlicerProviderData holds the configured client for resources and data sources.
type SlicerProviderData struct {
	Client *slicer.SlicerClient
}

func (p *SlicerProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "slicer"
	resp.Version = p.version
}

func (p *SlicerProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Slicer provider allows you to manage Slicer VMs and related resources.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "The Slicer API endpoint URL. Can also be set via the `SLICER_ENDPOINT` environment variable.",
				Optional:            true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "The bearer token for Slicer API authentication. Can also be set via the `SLICER_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"timeout": schema.StringAttribute{
				MarkdownDescription: "HTTP client timeout (e.g., '30s', '1m'). Defaults to '30s'.",
				Optional:            true,
			},
			"insecure": schema.BoolAttribute{
				MarkdownDescription: "Skip TLS certificate verification. Defaults to false.",
				Optional:            true,
			},
		},
	}
}

func (p *SlicerProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data SlicerProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get endpoint from config or environment
	endpoint := os.Getenv("SLICER_ENDPOINT")
	if !data.Endpoint.IsNull() {
		endpoint = data.Endpoint.ValueString()
	}

	if endpoint == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("endpoint"),
			"Missing Slicer API Endpoint",
			"The provider cannot create the Slicer API client without an endpoint. "+
				"Either set the endpoint in the provider configuration or use the SLICER_ENDPOINT environment variable.",
		)
	}

	// Get token from config or environment
	token := os.Getenv("SLICER_TOKEN")
	if !data.Token.IsNull() {
		token = data.Token.ValueString()
	}

	if token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Missing Slicer API Token",
			"The provider cannot create the Slicer API client without a token. "+
				"Either set the token in the provider configuration or use the SLICER_TOKEN environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Parse timeout
	timeout := 30 * time.Second
	if !data.Timeout.IsNull() {
		parsed, err := time.ParseDuration(data.Timeout.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("timeout"),
				"Invalid Timeout Value",
				"Could not parse timeout value: "+err.Error(),
			)
			return
		}
		timeout = parsed
	}

	// Configure HTTP client
	transport := &http.Transport{}
	if !data.Insecure.IsNull() && data.Insecure.ValueBool() {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	// Create Slicer client
	userAgent := "terraform-provider-slicer/" + p.version
	client := slicer.NewSlicerClient(endpoint, token, userAgent, httpClient)

	tflog.Debug(ctx, "Configured Slicer client", map[string]interface{}{
		"endpoint": endpoint,
		"timeout":  timeout.String(),
	})

	providerData := &SlicerProviderData{
		Client: client,
	}

	resp.DataSourceData = providerData
	resp.ResourceData = providerData
}

func (p *SlicerProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewVMResource,
		NewExecResource,
		NewFileResource,
		NewSecretResource,
	}
}

func (p *SlicerProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewVMDataSource,
		NewVMsDataSource,
		NewHostgroupsDataSource,
		NewSecretDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &SlicerProvider{
			version: version,
		}
	}
}
