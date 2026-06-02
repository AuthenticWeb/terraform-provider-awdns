// Package provider implements the Terraform provider for Authentic Web DNS
// (awdns). It bridges the terraform-plugin-framework provider surface to the
// awdns-go SDK, which authenticates with the OAuth2 client-credentials grant
// against the AW external API.
//
// This is the scaffold: it exposes provider configuration and constructs the
// SDK client. Resources (e.g. awdns_dns_record) target the PROJECTED API
// surface and land in a later component once the upstream record endpoints
// exist.
package provider

import (
	"context"
	"os"

	awdns "github.com/AuthenticWeb/awdns-go"
	"github.com/AuthenticWeb/terraform-provider-awdns/internal/datasources"
	"github.com/AuthenticWeb/terraform-provider-awdns/internal/resources"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// version is the provider version reported in Metadata. The release binary's
// build-time version lives in package main; this constant identifies the
// provider schema generation and mirrors the awdns-go SDK version for now.
const version = "0.1.0"

// defaultBaseURL is the AW external API base URL used when base_url is unset.
//
// NOTE: awdns-go's NewClient expects the application ROOT (e.g.
// "https://api.authenticweb.com") and appends "/external/v1" itself, also
// hitting "/oauth/token" at the root. This default carries the "/external/v1"
// suffix to match the documented provider schema. The two must be reconciled
// before the API goes live — see README.md ("Known scaffold caveats").
const defaultBaseURL = "https://api.authenticweb.com/external/v1"

// defaultRequestTimeout is the default per-request timeout in seconds. It is
// surfaced in the schema for forward compatibility; the current SDK hard-codes
// a 30s timeout and exposes no setter, so it is not yet wired through.
const defaultRequestTimeout int64 = 30

// Environment-variable fallbacks for the provider credentials. A config
// attribute always wins; the env var is consulted only when the attribute is
// unset. These names are the provider's documented auth contract (see README
// "Authentication").
const (
	envClientID     = "AWDNS_CLIENT_ID"
	envClientSecret = "AWDNS_CLIENT_SECRET"
	envBaseURL      = "AWDNS_BASE_URL"
)

// Compile-time assertion that awdnsProvider implements the framework interface.
var _ provider.Provider = (*awdnsProvider)(nil)

// awdnsProvider is the provider implementation.
type awdnsProvider struct{}

// New returns a new awdns provider. Its signature is the factory expected by
// providerserver.Serve in package main.
func New() provider.Provider {
	return &awdnsProvider{}
}

// awdnsProviderModel maps the provider configuration block to Go values.
type awdnsProviderModel struct {
	ClientID       types.String `tfsdk:"client_id"`
	ClientSecret   types.String `tfsdk:"client_secret"`
	BaseURL        types.String `tfsdk:"base_url"`
	RequestTimeout types.Int64  `tfsdk:"request_timeout"`
}

func (p *awdnsProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "awdns"
	resp.Version = version
}

func (p *awdnsProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage Authentic Web DNS hosted zones and records via the AW external API.",
		Attributes: map[string]schema.Attribute{
			"client_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "OAuth2 client ID for the AW external API (client-credentials grant). May also be set with the `" + envClientID + "` environment variable; the argument takes precedence.",
			},
			"client_secret": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "OAuth2 client secret for the AW external API (client-credentials grant). May also be set with the `" + envClientSecret + "` environment variable; the argument takes precedence.",
			},
			"base_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Base URL of the AW external API. May also be set with the `" + envBaseURL + "` environment variable. Defaults to `" + defaultBaseURL + "`.",
			},
			"request_timeout": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Per-request timeout in seconds. Defaults to 30. Reserved for a future SDK option; not yet enforced by the client.",
			},
		},
	}
}

// Configure reads the provider block, applies defaults, and constructs the
// awdns SDK client, making it available to resources and data sources.
func (p *awdnsProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config awdnsProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The client must be built now, so reject values that are still unknown at
	// configuration time (e.g. sourced from another resource's pending output).
	if config.ClientID.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("client_id"),
			"Unknown client_id",
			"The provider cannot create the AW DNS client because client_id is unknown at configuration time.")
	}
	if config.ClientSecret.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("client_secret"),
			"Unknown client_secret",
			"The provider cannot create the AW DNS client because client_secret is unknown at configuration time.")
	}
	if config.BaseURL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("base_url"),
			"Unknown base_url",
			"The provider cannot create the AW DNS client because base_url is unknown at configuration time.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	clientID, clientSecret, baseURL, diags := resolveCredentials(config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// request_timeout is accepted but not yet wired: NewClient takes no timeout
	// argument and the SDK exposes no setter. Acknowledge the field so the
	// intent is explicit until a WithTimeout-style option lands in the SDK.
	_ = config.RequestTimeout

	client := awdns.NewClient(baseURL, clientID, clientSecret)

	resp.DataSourceData = client
	resp.ResourceData = client
}

// resolveCredentials applies the provider's config-then-environment precedence
// for client_id, client_secret, and base_url, and applies the base_url default.
// A non-null config attribute always wins; otherwise the matching AWDNS_* env
// var is used. It returns the resolved values plus diagnostics for any required
// credential still empty by both paths. It is pure (constructs no client) so the
// precedence and the "missing credential" errors are unit-testable without
// standing up a framework ConfigureRequest.
func resolveCredentials(config awdnsProviderModel) (clientID, clientSecret, baseURL string, diags diag.Diagnostics) {
	clientID = os.Getenv(envClientID)
	if !config.ClientID.IsNull() {
		clientID = config.ClientID.ValueString()
	}
	clientSecret = os.Getenv(envClientSecret)
	if !config.ClientSecret.IsNull() {
		clientSecret = config.ClientSecret.ValueString()
	}
	baseURL = os.Getenv(envBaseURL)
	if !config.BaseURL.IsNull() {
		baseURL = config.BaseURL.ValueString()
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	if clientID == "" {
		diags.AddAttributeError(path.Root("client_id"),
			"Missing client_id",
			"Set the provider \"client_id\" argument or the "+envClientID+" environment variable.")
	}
	if clientSecret == "" {
		diags.AddAttributeError(path.Root("client_secret"),
			"Missing client_secret",
			"Set the provider \"client_secret\" argument or the "+envClientSecret+" environment variable.")
	}
	return clientID, clientSecret, baseURL, diags
}

// Resources returns the provider's managed resources: awdns_dns_record, which
// manages a DNS record within a domain via the projected record API.
func (p *awdnsProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewDNSRecordResource,
	}
}

// DataSources returns the provider's data sources: a single-domain lookup
// (awdns_domain) and a filtered list (awdns_domains). Both target the live
// domain endpoints of the AW external API.
func (p *awdnsProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		datasources.NewDomainDataSource,
		datasources.NewDomainsDataSource,
	}
}
