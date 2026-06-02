package datasources

import (
	"context"
	"fmt"

	awdns "github.com/AuthenticWeb/awdns-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*awdnsDomainsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*awdnsDomainsDataSource)(nil)
)

// NewDomainsDataSource is the factory registered in the provider's
// DataSources() list. It returns a fresh awdns_domains data source.
func NewDomainsDataSource() datasource.DataSource {
	return &awdnsDomainsDataSource{}
}

// awdnsDomainsDataSource lists all domains, optionally narrowed by a filter.
type awdnsDomainsDataSource struct {
	client *awdns.Client
}

// domainsModel is the Terraform state shape for the list data source.
type domainsModel struct {
	Filter  *domainsFilterModel `tfsdk:"filter"`
	Domains []domainModel       `tfsdk:"domains"`
}

// domainsFilterModel is the optional client-side filter. A null field means
// "don't filter on this dimension"; the API itself exposes no query filter, so
// filtering is applied locally over the full listing.
type domainsFilterModel struct {
	Status      types.Int64  `tfsdk:"status"`
	DNSProvider types.String `tfsdk:"dns_provider"`
}

func (d *awdnsDomainsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domains"
}

func (d *awdnsDomainsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List all Authentic Web DNS domains, optionally narrowed by an in-provider `filter`. The upstream API has no query filter, so the filter is applied client-side over the full paginated listing.",
		Attributes: map[string]schema.Attribute{
			"filter": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Optional client-side filter. Each field is ANDed; omit a field to leave that dimension unfiltered.",
				Attributes: map[string]schema.Attribute{
					"status": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Keep only domains with this upstream status code.",
					},
					"dns_provider": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Keep only domains served by this DNS provider.",
					},
				},
			},
			"domains": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The matched domains.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Numeric domain ID.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Domain name as registered upstream.",
						},
						"fqdn": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Fully-qualified domain name.",
						},
						"tld": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Top-level domain.",
						},
						"status": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Upstream domain status code.",
						},
						"dns_provider": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "DNS provider backing the zone.",
						},
						"nameserver_list": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Authoritative nameservers for the domain.",
						},
						"use_our_zone": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the domain uses an Authentic-Web-hosted zone.",
						},
					},
				},
			},
		},
	}
}

func (d *awdnsDomainsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*awdns.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *awdns.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	d.client = client
}

func (d *awdnsDomainsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config domainsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured AW DNS client",
			"The awdns_domains data source was used before the provider was configured. This is a bug in the provider.",
		)
		return
	}

	all, err := listAllDomains(ctx, d.client)
	if err != nil {
		resp.Diagnostics.AddError("Error listing AW DNS domains", err.Error())
		return
	}

	state := domainsModel{Filter: config.Filter}
	for i := range all {
		if !domainMatchesFilter(&all[i], config.Filter) {
			continue
		}
		m, diags := flattenDomain(ctx, &all[i])
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.Domains = append(state.Domains, m)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// domainMatchesFilter reports whether d passes the optional filter. A nil
// filter (or an unset field within it) matches everything on that dimension.
func domainMatchesFilter(d *awdns.Domain, f *domainsFilterModel) bool {
	if f == nil {
		return true
	}
	if !f.Status.IsNull() && !f.Status.IsUnknown() && int64(d.Status) != f.Status.ValueInt64() {
		return false
	}
	if !f.DNSProvider.IsNull() && !f.DNSProvider.IsUnknown() && d.DNSProvider != f.DNSProvider.ValueString() {
		return false
	}
	return true
}
