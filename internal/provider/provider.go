// Package provider implements the Hostereo Terraform provider.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/InterhostNetworks/terraform-provider-hostereo/internal/client"
)

var _ provider.Provider = (*hostereoProvider)(nil)

type hostereoProvider struct {
	version string
}

type providerModel struct {
	APIKey   types.String `tfsdk:"api_key"`
	Endpoint types.String `tfsdk:"endpoint"`
}

// New returns a provider factory.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &hostereoProvider{version: version} }
}

func (p *hostereoProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "hostereo"
	resp.Version = p.version
}

func (p *hostereoProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage [Hostereo](https://www.hostereo.com) VPS instances, projects and tags through the Hostereo External API.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				MarkdownDescription: "Hostereo API key (`hst_live_…`). Create one in the console under **Account → API Keys**. Keys are bound to source IPs: allow the IP Terraform runs from. Can also be set with `HOSTEREO_API_KEY`.",
				Optional:            true,
				Sensitive:           true,
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "API base URL. Defaults to `" + client.DefaultEndpoint + "`. Can also be set with `HOSTEREO_ENDPOINT`.",
				Optional:            true,
			},
		},
	}
}

func (p *hostereoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiKey := os.Getenv("HOSTEREO_API_KEY")
	if !cfg.APIKey.IsNull() && !cfg.APIKey.IsUnknown() {
		apiKey = cfg.APIKey.ValueString()
	}
	endpoint := os.Getenv("HOSTEREO_ENDPOINT")
	if !cfg.Endpoint.IsNull() && !cfg.Endpoint.IsUnknown() {
		endpoint = cfg.Endpoint.ValueString()
	}
	// The catalogue is public, so a missing key is only an error once a
	// resource or authenticated data source is used.
	c := client.New(endpoint, apiKey, "terraform-provider-hostereo/"+p.version)
	resp.DataSourceData = &providerData{client: c, hasKey: apiKey != ""}
	resp.ResourceData = &providerData{client: c, hasKey: apiKey != ""}
}

func (p *hostereoProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewInstanceResource,
		NewProjectResource,
		NewTagResource,
	}
}

func (p *hostereoProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewRegionsDataSource,
		NewPlansDataSource,
		NewImagesDataSource,
		NewApplicationsDataSource,
		NewInstanceDataSource,
	}
}

type providerData struct {
	client *client.Client
	hasKey bool
}

const missingKeyDetail = "Set api_key in the provider block or the HOSTEREO_API_KEY environment variable. Create a key in the Hostereo console under Account → API Keys and allow the IP address Terraform runs from."
