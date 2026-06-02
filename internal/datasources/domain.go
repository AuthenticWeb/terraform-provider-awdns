// Package datasources implements the read-only data sources for the awdns
// Terraform provider: awdns_domain (single-domain lookup) and awdns_domains
// (list with optional filter). Both bridge the terraform-plugin-framework
// data-source surface to the awdns-go SDK client wired up by the provider's
// Configure step.
//
// The domain endpoints (list/show) are the one part of the AW external API that
// is real today; the record endpoints remain projected. These data sources
// therefore target the live domain surface.
package datasources

import (
	"context"
	"fmt"
	"strconv"

	awdns "github.com/AuthenticWeb/awdns-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Interface assertions: awdnsDomainDataSource implements the base data-source
// contract plus Configure (to receive the shared *awdns.Client).
var (
	_ datasource.DataSource              = (*awdnsDomainDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*awdnsDomainDataSource)(nil)
)

// NewDomainDataSource is the factory registered in the provider's DataSources()
// list. It returns a fresh awdns_domain data source.
func NewDomainDataSource() datasource.DataSource {
	return &awdnsDomainDataSource{}
}

// awdnsDomainDataSource looks up exactly one domain, by numeric id or by fqdn.
type awdnsDomainDataSource struct {
	client *awdns.Client
}

// domainModel is the Terraform state shape for a single domain. It is shared:
// it is the whole state of awdns_domain and the element type of the awdns_domains
// "domains" list, so the tfsdk tags must match both schemas' attribute names.
//
// id and fqdn are Optional+Computed: the caller supplies exactly one as the
// lookup key, and Read fills in the other from the matched domain. (A data
// source may only write back an Optional attribute that is also Computed, hence
// both carry Computed even though they are the user-facing lookup keys.)
type domainModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	FQDN           types.String `tfsdk:"fqdn"`
	TLD            types.String `tfsdk:"tld"`
	Status         types.Int64  `tfsdk:"status"`
	DNSProvider    types.String `tfsdk:"dns_provider"`
	NameserverList types.List   `tfsdk:"nameserver_list"`
	UseOurZone     types.Bool   `tfsdk:"use_our_zone"`
}

func (d *awdnsDomainDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (d *awdnsDomainDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a single Authentic Web DNS domain by its numeric `id` or its `fqdn`. Exactly one of the two must be set.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Numeric domain ID. Set this OR `fqdn` (not both). Computed when looked up by `fqdn`.",
			},
			"fqdn": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Fully-qualified domain name. Set this OR `id` (not both). Computed when looked up by `id`.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Domain name as registered upstream.",
			},
			"tld": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Top-level domain.",
			},
			"status": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Upstream domain status code (e.g. 7 = active, 9 = expired). See the awdns-go DomainStatus* constants.",
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
	}
}

func (d *awdnsDomainDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// ProviderData is nil during the provider's own validate/early phases; the
	// framework calls Configure again once the client exists.
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

func (d *awdnsDomainDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config domainModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured AW DNS client",
			"The awdns_domain data source was used before the provider was configured. This is a bug in the provider.",
		)
		return
	}

	idSet := !config.ID.IsNull() && !config.ID.IsUnknown()
	fqdnSet := !config.FQDN.IsNull() && !config.FQDN.IsUnknown()

	// Exactly one lookup key. idSet == fqdnSet catches both "neither" and "both".
	if idSet == fqdnSet {
		resp.Diagnostics.AddError(
			"Invalid awdns_domain lookup",
			"Exactly one of \"id\" or \"fqdn\" must be set on the awdns_domain data source.",
		)
		return
	}

	var domain *awdns.Domain
	switch {
	case idSet:
		raw := config.ID.ValueString()
		id, err := strconv.Atoi(raw)
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("id"),
				"Invalid domain id",
				fmt.Sprintf("id must be a base-10 integer, got %q: %s", raw, err),
			)
			return
		}
		domain, err = d.client.GetDomain(ctx, id)
		if err != nil {
			if awdns.IsNotFound(err) {
				resp.Diagnostics.AddError("Domain not found", fmt.Sprintf("No AW DNS domain with id %d.", id))
				return
			}
			resp.Diagnostics.AddError("Error reading AW DNS domain", err.Error())
			return
		}
	case fqdnSet:
		want := config.FQDN.ValueString()
		all, err := listAllDomains(ctx, d.client)
		if err != nil {
			resp.Diagnostics.AddError("Error listing AW DNS domains", err.Error())
			return
		}
		for i := range all {
			if all[i].FQDN == want {
				domain = &all[i]
				break
			}
		}
		if domain == nil {
			resp.Diagnostics.AddError("Domain not found", fmt.Sprintf("No AW DNS domain with fqdn %q.", want))
			return
		}
	}

	state, diags := flattenDomain(ctx, domain)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// flattenDomain maps an SDK Domain onto the Terraform state model. The numeric
// SDK id is rendered as a string to match the schema's string id attribute.
func flattenDomain(ctx context.Context, d *awdns.Domain) (domainModel, diag.Diagnostics) {
	ns, diags := types.ListValueFrom(ctx, types.StringType, d.NameserverList)
	return domainModel{
		ID:             types.StringValue(strconv.Itoa(d.ID)),
		Name:           types.StringValue(d.Name),
		FQDN:           types.StringValue(d.FQDN),
		TLD:            types.StringValue(d.TLD),
		Status:         types.Int64Value(int64(d.Status)),
		DNSProvider:    types.StringValue(d.DNSProvider),
		NameserverList: ns,
		UseOurZone:     types.BoolValue(d.UseOurZone),
	}, diags
}

// listAllDomains pages through the entire domain list, following the Laravel
// paginator (current_page/last_page) until exhausted. perPage 0 defers to the
// server default. The break conditions guard against a missing/zero last_page
// and an empty page so a malformed paginator can never spin forever.
func listAllDomains(ctx context.Context, client *awdns.Client) ([]awdns.Domain, error) {
	var all []awdns.Domain
	for page := 1; ; page++ {
		resp, err := client.ListDomains(ctx, page, 0)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Data...)
		if len(resp.Data) == 0 || resp.LastPage <= 0 || resp.CurrentPage >= resp.LastPage {
			break
		}
	}
	return all, nil
}
