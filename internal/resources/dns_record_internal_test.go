package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	awdns "github.com/AuthenticWeb/awdns-go"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// newRecordTestServer stands in for the projected AW record API: a plain OAuth
// token endpoint plus enveloped create/show/delete handlers nested under a
// domain (/external/v1/domains/5/records[/12]).
func newRecordTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	const record12 = `{"status":"success","data":` +
		`{"id":12,"domain_id":5,"subdomain":"www","type":"A","ttl":3600,` +
		`"values":["203.0.113.10","203.0.113.11"],"service_record_id":"ns1-abc","note":"web"}}`

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
	})
	// Collection: POST creates and echoes the canonical record.
	mux.HandleFunc("/external/v1/domains/5/records", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"status":"error","message":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte(record12))
	})
	// Item: GET shows it, DELETE removes it; unknown ids 404.
	mux.HandleFunc("/external/v1/domains/5/records/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/external/v1/domains/5/records/12" {
			http.Error(w, `{"status":"error","message":"not found"}`, http.StatusNotFound)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(record12))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCreateRecordAndFlatten(t *testing.T) {
	srv := newRecordTestServer(t)
	client := awdns.NewClient(srv.URL, "id", "secret")

	rec, err := client.CreateRecord(context.Background(), 5, awdns.RecordInput{
		DomainID:  5,
		Subdomain: "www",
		Type:      awdns.RecordTypeA,
		TTL:       3600,
		Values:    []string{"203.0.113.10", "203.0.113.11"},
		Note:      "web",
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}

	m, diags := flattenRecord(context.Background(), rec)
	if diags.HasError() {
		t.Fatalf("flattenRecord diagnostics: %v", diags)
	}
	if got := m.ID.ValueString(); got != "12" {
		t.Errorf("ID = %q, want \"12\" (int rendered as string)", got)
	}
	if got := m.DomainID.ValueString(); got != "5" {
		t.Errorf("DomainID = %q, want \"5\"", got)
	}
	if got := m.Subdomain.ValueString(); got != "www" {
		t.Errorf("Subdomain = %q, want \"www\"", got)
	}
	if got := m.Type.ValueString(); got != "A" {
		t.Errorf("Type = %q, want \"A\"", got)
	}
	if got := m.TTL.ValueInt64(); got != 3600 {
		t.Errorf("TTL = %d, want 3600", got)
	}
	if got := m.ServiceRecordID.ValueString(); got != "ns1-abc" {
		t.Errorf("ServiceRecordID = %q, want \"ns1-abc\"", got)
	}
	if got := m.Note.ValueString(); got != "web" {
		t.Errorf("Note = %q, want \"web\"", got)
	}
	var values []string
	if d := m.Values.ElementsAs(context.Background(), &values, false); d.HasError() {
		t.Fatalf("values ElementsAs: %v", d)
	}
	if len(values) != 2 || values[0] != "203.0.113.10" || values[1] != "203.0.113.11" {
		t.Errorf("values = %v, want [203.0.113.10 203.0.113.11]", values)
	}
}

func TestGetAndDeleteRecord(t *testing.T) {
	srv := newRecordTestServer(t)
	client := awdns.NewClient(srv.URL, "id", "secret")

	if _, err := client.GetRecord(context.Background(), 5, 12); err != nil {
		t.Fatalf("GetRecord(5,12): %v", err)
	}
	if err := client.DeleteRecord(context.Background(), 5, 12); err != nil {
		t.Fatalf("DeleteRecord(5,12): %v", err)
	}
}

func TestGetRecordNotFound(t *testing.T) {
	srv := newRecordTestServer(t)
	client := awdns.NewClient(srv.URL, "id", "secret")

	_, err := client.GetRecord(context.Background(), 5, 999)
	if err == nil {
		t.Fatal("expected error for missing record, got nil")
	}
	if !awdns.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

func TestRecordTypeValidator(t *testing.T) {
	tests := []struct {
		val     string
		wantErr bool
	}{
		{"A", false},
		{"AAAA", false},
		{"CNAME", false},
		{"MX", false},
		{"TXT", false},
		{"NS", false},
		{"SRV", false},
		{"CAA", false},
		{"ALIAS", false},
		{"SOA", true},   // zone-level, deliberately excluded
		{"a", true},     // case-sensitive
		{"BOGUS", true}, // unknown
		{"", true},      // empty string is not a valid type
	}
	for _, tc := range tests {
		t.Run(tc.val, func(t *testing.T) {
			resp := &validator.StringResponse{}
			recordTypeValidator{}.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("type"),
				ConfigValue: types.StringValue(tc.val),
			}, resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Errorf("type %q: HasError = %v, want %v (%v)", tc.val, got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestRecordTypeValidatorSkipsNullAndUnknown(t *testing.T) {
	for name, cv := range map[string]types.String{
		"null":    types.StringNull(),
		"unknown": types.StringUnknown(),
	} {
		t.Run(name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			recordTypeValidator{}.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("type"),
				ConfigValue: cv,
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Errorf("%s value should not error, got %v", name, resp.Diagnostics)
			}
		})
	}
}

func TestParseRecordImportID(t *testing.T) {
	tests := []struct {
		id                 string
		wantDom, wantRec   string
		wantErr            bool
	}{
		{"5/12", "5", "12", false},
		{"100/2000", "100", "2000", false},
		{"5", "", "", true},          // missing separator
		{"5/12/3", "", "", true},     // too many parts
		{"/12", "", "", true},        // empty domain
		{"5/", "", "", true},         // empty record
		{"abc/12", "", "", true},     // non-integer domain
		{"5/def", "", "", true},      // non-integer record
		{"", "", "", true},           // empty
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			dom, rec, err := parseRecordImportID(tc.id)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseRecordImportID(%q) err = %v, wantErr %v", tc.id, err, tc.wantErr)
			}
			if !tc.wantErr && (dom != tc.wantDom || rec != tc.wantRec) {
				t.Errorf("parseRecordImportID(%q) = (%q,%q), want (%q,%q)", tc.id, dom, rec, tc.wantDom, tc.wantRec)
			}
		})
	}
}

func TestDNSRecordMetadata(t *testing.T) {
	resp := &resource.MetadataResponse{}
	NewDNSRecordResource().Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "awdns"}, resp)
	if resp.TypeName != "awdns_dns_record" {
		t.Errorf("TypeName = %q, want \"awdns_dns_record\"", resp.TypeName)
	}
}

func TestDNSRecordSchema(t *testing.T) {
	resp := &resource.SchemaResponse{}
	NewDNSRecordResource().Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "domain_id", "subdomain", "type", "ttl", "values", "note", "service_record_id"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("awdns_dns_record schema missing attribute %q", name)
		}
	}
	if !attrs["domain_id"].IsRequired() {
		t.Error("domain_id should be required")
	}
	if !attrs["subdomain"].IsRequired() {
		t.Error("subdomain should be required")
	}
	if !attrs["type"].IsRequired() {
		t.Error("type should be required")
	}
	if !attrs["values"].IsRequired() {
		t.Error("values should be required")
	}
	if !attrs["id"].IsComputed() || attrs["id"].IsOptional() || attrs["id"].IsRequired() {
		t.Error("id should be computed-only")
	}
	if !attrs["service_record_id"].IsComputed() {
		t.Error("service_record_id should be computed")
	}
	// ttl and note are optional with a computed fallback (default / server value).
	for _, name := range []string{"ttl", "note"} {
		if !attrs[name].IsOptional() || !attrs[name].IsComputed() {
			t.Errorf("%q should be optional+computed", name)
		}
	}
}
