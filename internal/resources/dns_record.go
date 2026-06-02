// Package resources implements the managed resources for the awdns Terraform
// provider. Today that is a single resource, awdns_dns_record, which bridges the
// terraform-plugin-framework resource surface to the awdns-go SDK client wired
// up by the provider's Configure step.
//
// NOTE: the record endpoints target the PROJECTED API surface — upstream
// currently only exposes domain list/show (consumed by the awdns_domain data
// sources). The CRUD here follows the planned RecordController shape nested
// under a domain: /external/v1/domains/{domainID}/records[/{recordID}]. It is
// wired and unit-tested now so it is ready the moment the upstream endpoints land.
package resources

import (
	"context"
	"fmt"
	"strconv"

	awdns "github.com/AuthenticWeb/awdns-go"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// defaultRecordTTL is the TTL applied when the user omits ttl. 3600s (1h) is the
// conventional default and matches the provider design findings.
const defaultRecordTTL int64 = 3600

// Interface assertions: awdnsDNSRecordResource implements the base resource
// contract plus Configure (to receive the shared *awdns.Client) and ImportState
// (to support `terraform import` by "domain_id/record_id").
var (
	_ resource.Resource                = (*awdnsDNSRecordResource)(nil)
	_ resource.ResourceWithConfigure   = (*awdnsDNSRecordResource)(nil)
	_ resource.ResourceWithImportState = (*awdnsDNSRecordResource)(nil)
)

// NewDNSRecordResource is the factory registered in the provider's Resources()
// list. It returns a fresh awdns_dns_record resource.
func NewDNSRecordResource() resource.Resource {
	return &awdnsDNSRecordResource{}
}

// awdnsDNSRecordResource manages a single DNS record within a domain.
type awdnsDNSRecordResource struct {
	client *awdns.Client
}

// dnsRecordModel is the Terraform state shape for one DNS record.
//
// id and domain_id are the SDK's numeric ids rendered as strings to match the
// awdns_domain data source (whose id is also a string), so a record can be
// wired straight to a looked-up domain: domain_id = data.awdns_domain.x.id.
type dnsRecordModel struct {
	ID              types.String `tfsdk:"id"`
	DomainID        types.String `tfsdk:"domain_id"`
	Subdomain       types.String `tfsdk:"subdomain"`
	Type            types.String `tfsdk:"type"`
	TTL             types.Int64  `tfsdk:"ttl"`
	Values          types.List   `tfsdk:"values"`
	Note            types.String `tfsdk:"note"`
	ServiceRecordID types.String `tfsdk:"service_record_id"`
}

func (r *awdnsDNSRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *awdnsDNSRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a single DNS record within an Authentic Web DNS domain. Records are addressed by their parent `domain_id`; changing it forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Numeric record ID assigned by the API, rendered as a string.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"domain_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Numeric ID of the domain that owns this record (e.g. `data.awdns_domain.example.id`). Changing it moves the record to a different domain and forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"subdomain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record subdomain / host label, relative to the domain (e.g. `www`, or `@` for the apex).",
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "DNS record type. One of: A, AAAA, CNAME, MX, TXT, NS, SRV, CAA, ALIAS.",
				Validators: []validator.String{
					recordTypeValidator{},
				},
			},
			"ttl": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(defaultRecordTTL),
				MarkdownDescription: "Time-to-live in seconds. Defaults to 3600.",
			},
			"values": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Record values (rdata). A list so multi-value record sets (e.g. several A records, or MX with priorities) are expressed natively.",
			},
			"note": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Optional free-text annotation stored alongside the record.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_record_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Opaque identifier assigned by the backing DNS service (e.g. NS1) for this record. Server-managed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *awdnsDNSRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// ProviderData is nil during the provider's own validate/early phases; the
	// framework calls Configure again once the client exists.
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*awdns.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *awdns.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.client = client
}

func (r *awdnsDNSRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.configured(&resp.Diagnostics) {
		return
	}

	domainID, ok := r.parseDomainID(plan.DomainID, &resp.Diagnostics)
	if !ok {
		return
	}
	input, ok := r.buildInput(ctx, domainID, plan, &resp.Diagnostics)
	if !ok {
		return
	}

	rec, err := r.client.CreateRecord(ctx, domainID, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating AW DNS record", err.Error())
		return
	}

	state, diags := flattenRecord(ctx, rec)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Preserve the user-supplied domain_id: the create call is already scoped by
	// it, and the projected API may not echo domain_id back on the record.
	state.DomainID = plan.DomainID
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *awdnsDNSRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !r.configured(&resp.Diagnostics) {
		return
	}

	domainID, ok := r.parseDomainID(state.DomainID, &resp.Diagnostics)
	if !ok {
		return
	}
	recordID, ok := r.parseRecordID(state.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	rec, err := r.client.GetRecord(ctx, domainID, recordID)
	if err != nil {
		if awdns.IsNotFound(err) {
			// The record was deleted out-of-band; drop it from state so a
			// subsequent apply re-creates it rather than erroring.
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading AW DNS record", err.Error())
		return
	}

	newState, diags := flattenRecord(ctx, rec)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	newState.DomainID = state.DomainID // GetRecord is scoped by the known domain.
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *awdnsDNSRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !r.configured(&resp.Diagnostics) {
		return
	}

	// domain_id is RequiresReplace, so on an in-place update it equals the prior
	// state; the record id never changes and is carried from state.
	domainID, ok := r.parseDomainID(plan.DomainID, &resp.Diagnostics)
	if !ok {
		return
	}
	recordID, ok := r.parseRecordID(state.ID, &resp.Diagnostics)
	if !ok {
		return
	}
	input, ok := r.buildInput(ctx, domainID, plan, &resp.Diagnostics)
	if !ok {
		return
	}

	rec, err := r.client.UpdateRecord(ctx, domainID, recordID, input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating AW DNS record", err.Error())
		return
	}

	newState, diags := flattenRecord(ctx, rec)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	newState.DomainID = plan.DomainID
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *awdnsDNSRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !r.configured(&resp.Diagnostics) {
		return
	}

	domainID, ok := r.parseDomainID(state.DomainID, &resp.Diagnostics)
	if !ok {
		return
	}
	recordID, ok := r.parseRecordID(state.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	if err := r.client.DeleteRecord(ctx, domainID, recordID); err != nil && !awdns.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting AW DNS record", err.Error())
	}
}

// ImportState supports `terraform import awdns_dns_record.x DOMAIN_ID/RECORD_ID`.
// It seeds domain_id and id; the next Read fills in the rest.
func (r *awdnsDNSRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	domainID, recordID, err := parseRecordImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain_id"), domainID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), recordID)...)
}

// configured reports whether the SDK client was wired by the provider, adding a
// diagnostic when it was not (a provider bug, e.g. a CRUD call before Configure).
func (r *awdnsDNSRecordResource) configured(diags *diag.Diagnostics) bool {
	if r.client == nil {
		diags.AddError(
			"Unconfigured AW DNS client",
			"The awdns_dns_record resource was used before the provider was configured. This is a bug in the provider.",
		)
		return false
	}
	return true
}

// parseDomainID converts the string domain_id attribute to the int the SDK wants.
func (r *awdnsDNSRecordResource) parseDomainID(v types.String, diags *diag.Diagnostics) (int, bool) {
	id, err := strconv.Atoi(v.ValueString())
	if err != nil {
		diags.AddAttributeError(path.Root("domain_id"), "Invalid domain_id",
			fmt.Sprintf("domain_id must be a base-10 integer, got %q: %s", v.ValueString(), err))
		return 0, false
	}
	return id, true
}

// parseRecordID converts the computed string id to the int the SDK wants.
func (r *awdnsDNSRecordResource) parseRecordID(v types.String, diags *diag.Diagnostics) (int, bool) {
	id, err := strconv.Atoi(v.ValueString())
	if err != nil {
		diags.AddError("Invalid record id",
			fmt.Sprintf("record id must be a base-10 integer, got %q: %s", v.ValueString(), err))
		return 0, false
	}
	return id, true
}

// buildInput assembles the SDK RecordInput from a plan model, converting the
// values list. domainID is passed in already parsed.
func (r *awdnsDNSRecordResource) buildInput(ctx context.Context, domainID int, plan dnsRecordModel, diags *diag.Diagnostics) (awdns.RecordInput, bool) {
	var values []string
	diags.Append(plan.Values.ElementsAs(ctx, &values, false)...)
	if diags.HasError() {
		return awdns.RecordInput{}, false
	}
	return awdns.RecordInput{
		DomainID:  domainID,
		Subdomain: plan.Subdomain.ValueString(),
		Type:      plan.Type.ValueString(),
		TTL:       int(plan.TTL.ValueInt64()),
		Values:    values,
		Note:      plan.Note.ValueString(),
	}, true
}

// flattenRecord maps an SDK Record onto the Terraform state model. The numeric
// SDK ids are rendered as strings to match the schema. The caller is expected to
// overwrite DomainID with the known request-scoped value (see Create/Read).
func flattenRecord(ctx context.Context, rec *awdns.Record) (dnsRecordModel, diag.Diagnostics) {
	values, diags := types.ListValueFrom(ctx, types.StringType, rec.Values)
	return dnsRecordModel{
		ID:              types.StringValue(strconv.Itoa(rec.ID)),
		DomainID:        types.StringValue(strconv.Itoa(rec.DomainID)),
		Subdomain:       types.StringValue(rec.Subdomain),
		Type:            types.StringValue(rec.Type),
		TTL:             types.Int64Value(int64(rec.TTL)),
		Values:          values,
		Note:            types.StringValue(rec.Note),
		ServiceRecordID: types.StringValue(rec.ServiceRecordID),
	}, diags
}
