package resources

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	awdns "github.com/AuthenticWeb/awdns-go"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// validRecordTypes is the set of DNS record types the resource accepts, per the
// component spec (A/AAAA/CNAME/MX/TXT/NS/SRV/CAA/ALIAS). It deliberately excludes
// SOA: the apex SOA is zone-level and not a user-managed record in this API.
// Values reference the awdns-go constants so the two stay in lockstep.
var validRecordTypes = []string{
	awdns.RecordTypeA,
	awdns.RecordTypeAAAA,
	awdns.RecordTypeCNAME,
	awdns.RecordTypeMX,
	awdns.RecordTypeTXT,
	awdns.RecordTypeNS,
	awdns.RecordTypeSRV,
	awdns.RecordTypeCAA,
	awdns.RecordTypeALIAS,
}

// recordTypeValidator rejects, at plan time, any `type` value outside
// validRecordTypes — so an invalid type fails `terraform plan` rather than
// surfacing as an opaque API error at apply time.
type recordTypeValidator struct{}

var _ validator.String = recordTypeValidator{}

func (v recordTypeValidator) Description(_ context.Context) string {
	return fmt.Sprintf("DNS record type must be one of: %s", strings.Join(validRecordTypes, ", "))
}

func (v recordTypeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v recordTypeValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	// Null/unknown are not this validator's concern: required-ness is enforced by
	// the schema, and an unknown (interpolated) value is validated post-apply.
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	got := req.ConfigValue.ValueString()
	for _, t := range validRecordTypes {
		if got == t {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid DNS record type",
		fmt.Sprintf("type %q is not supported; must be one of: %s.", got, strings.Join(validRecordTypes, ", ")),
	)
}

// parseRecordImportID splits a `terraform import` ID of the form
// "domain_id/record_id" into its two integer-valued string parts. Both must be
// present and base-10 integers; anything else is a usage error.
func parseRecordImportID(id string) (domainID, recordID string, err error) {
	parts := strings.Split(id, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("import ID %q must be in the form \"domain_id/record_id\"", id)
	}
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return "", "", fmt.Errorf("domain_id %q in import ID must be a base-10 integer", parts[0])
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return "", "", fmt.Errorf("record_id %q in import ID must be a base-10 integer", parts[1])
	}
	return parts[0], parts[1], nil
}
