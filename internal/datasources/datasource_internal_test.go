package datasources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	awdns "github.com/AuthenticWeb/awdns-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// newTestServer stands in for the AW external API: a plain (non-enveloped)
// OAuth token endpoint plus enveloped domain list/show endpoints. The list is
// paged (2 pages, 3 domains total) so listAllDomains' pagination is exercised.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	const page1 = `{"status":"success","data":{"data":[` +
		`{"id":1,"name":"example","fqdn":"example.com","tld":"com","status":7,"dns_provider":"ns1","nameserver_list":["a.ns","b.ns"],"use_our_zone":true},` +
		`{"id":2,"name":"foo","fqdn":"foo.org","tld":"org","status":1,"dns_provider":"route53","nameserver_list":[],"use_our_zone":false}` +
		`],"current_page":1,"last_page":2,"per_page":2,"total":3}}`
	const page2 = `{"status":"success","data":{"data":[` +
		`{"id":3,"name":"bar","fqdn":"bar.net","tld":"net","status":9,"dns_provider":"ns1","nameserver_list":["c.ns"],"use_our_zone":true}` +
		`],"current_page":2,"last_page":2,"per_page":2,"total":3}}`
	const domain7 = `{"status":"success","data":` +
		`{"id":7,"name":"seven","fqdn":"seven.io","tld":"io","status":7,"dns_provider":"ns1","nameserver_list":["x.ns"],"use_our_zone":false}}`

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/external/v1/domains", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(page2))
			return
		}
		_, _ = w.Write([]byte(page1))
	})
	mux.HandleFunc("/external/v1/domains/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/external/v1/domains/7" {
			_, _ = w.Write([]byte(domain7))
			return
		}
		http.Error(w, `{"status":"error","message":"not found"}`, http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestListAllDomainsPagination(t *testing.T) {
	srv := newTestServer(t)
	client := awdns.NewClient(srv.URL, "id", "secret")

	all, err := listAllDomains(context.Background(), client)
	if err != nil {
		t.Fatalf("listAllDomains: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 domains across 2 pages, got %d", len(all))
	}
	wantIDs := []int{1, 2, 3}
	for i, w := range wantIDs {
		if all[i].ID != w {
			t.Errorf("domain[%d].ID = %d, want %d", i, all[i].ID, w)
		}
	}
}

func TestGetDomainAndFlatten(t *testing.T) {
	srv := newTestServer(t)
	client := awdns.NewClient(srv.URL, "id", "secret")

	d, err := client.GetDomain(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetDomain(7): %v", err)
	}

	m, diags := flattenDomain(context.Background(), d)
	if diags.HasError() {
		t.Fatalf("flattenDomain diagnostics: %v", diags)
	}
	if got := m.ID.ValueString(); got != "7" {
		t.Errorf("ID = %q, want \"7\" (int rendered as string)", got)
	}
	if got := m.FQDN.ValueString(); got != "seven.io" {
		t.Errorf("FQDN = %q, want \"seven.io\"", got)
	}
	if got := m.Status.ValueInt64(); got != 7 {
		t.Errorf("Status = %d, want 7", got)
	}
	if got := m.UseOurZone.ValueBool(); got != false {
		t.Errorf("UseOurZone = %v, want false", got)
	}
	var ns []string
	if d := m.NameserverList.ElementsAs(context.Background(), &ns, false); d.HasError() {
		t.Fatalf("nameserver_list ElementsAs: %v", d)
	}
	if len(ns) != 1 || ns[0] != "x.ns" {
		t.Errorf("nameserver_list = %v, want [x.ns]", ns)
	}
}

func TestGetDomainNotFound(t *testing.T) {
	srv := newTestServer(t)
	client := awdns.NewClient(srv.URL, "id", "secret")

	_, err := client.GetDomain(context.Background(), 404)
	if err == nil {
		t.Fatal("expected error for missing domain, got nil")
	}
	if !awdns.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

func TestDomainMatchesFilter(t *testing.T) {
	d := &awdns.Domain{Status: 7, DNSProvider: "ns1"}

	i64 := func(v int64) types.Int64 { return types.Int64Value(v) }
	str := func(v string) types.String { return types.StringValue(v) }

	tests := []struct {
		name   string
		filter *domainsFilterModel
		want   bool
	}{
		{"nil filter matches", nil, true},
		{"empty filter matches", &domainsFilterModel{Status: types.Int64Null(), DNSProvider: types.StringNull()}, true},
		{"status match", &domainsFilterModel{Status: i64(7), DNSProvider: types.StringNull()}, true},
		{"status mismatch", &domainsFilterModel{Status: i64(9), DNSProvider: types.StringNull()}, false},
		{"provider match", &domainsFilterModel{Status: types.Int64Null(), DNSProvider: str("ns1")}, true},
		{"provider mismatch", &domainsFilterModel{Status: types.Int64Null(), DNSProvider: str("route53")}, false},
		{"both match (AND)", &domainsFilterModel{Status: i64(7), DNSProvider: str("ns1")}, true},
		{"one mismatch fails AND", &domainsFilterModel{Status: i64(7), DNSProvider: str("route53")}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := domainMatchesFilter(d, tc.filter); got != tc.want {
				t.Errorf("domainMatchesFilter = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDomainSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	NewDomainDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "fqdn", "name", "tld", "status", "dns_provider", "nameserver_list", "use_our_zone"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("awdns_domain schema missing attribute %q", name)
		}
	}
	// id and fqdn are the lookup keys: Optional (user supplies one) AND Computed
	// (the other is filled from the match).
	for _, name := range []string{"id", "fqdn"} {
		if !attrs[name].IsOptional() {
			t.Errorf("%q should be optional", name)
		}
		if !attrs[name].IsComputed() {
			t.Errorf("%q should be computed", name)
		}
	}
	if !attrs["name"].IsComputed() || attrs["name"].IsOptional() {
		t.Error("name should be computed-only")
	}
}

func TestDomainsSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	NewDomainsDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, name := range []string{"filter", "domains"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("awdns_domains schema missing attribute %q", name)
		}
	}
	if !resp.Schema.Attributes["filter"].IsOptional() {
		t.Error("filter should be optional")
	}
	if !resp.Schema.Attributes["domains"].IsComputed() {
		t.Error("domains should be computed")
	}
}

func TestMetadata(t *testing.T) {
	for _, tc := range []struct {
		newFn func() datasource.DataSource
		want  string
	}{
		{NewDomainDataSource, "awdns_domain"},
		{NewDomainsDataSource, "awdns_domains"},
	} {
		resp := &datasource.MetadataResponse{}
		tc.newFn().Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "awdns"}, resp)
		if resp.TypeName != tc.want {
			t.Errorf("TypeName = %q, want %q", resp.TypeName, tc.want)
		}
	}
}
