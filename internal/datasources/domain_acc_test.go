// Acceptance tests for the awdns data sources, exercised through a real
// in-process provider server with resource.Test(). They live in the external
// datasources_test package so they can import internal/provider (which imports
// internal/datasources) without an import cycle.
//
// resource.Test() is a no-op unless TF_ACC is set, so `go test ./...` never
// touches the network. testAccPreCheck additionally skips unless live AWDNS_*
// credentials are present — the AW external API domain endpoints must be
// reachable for these to run, which is why they are gated, not mocked: they
// assert end-to-end behavior against the real service.
package datasources_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/AuthenticWeb/terraform-provider-awdns/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testAccProtoV6ProviderFactories serves the in-process provider under the
// "awdns" address for the test's terraform runs.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"awdns": providerserver.NewProtocol6WithError(provider.New()),
}

func testAccPreCheck(t *testing.T) {
	for _, k := range []string{"AWDNS_CLIENT_ID", "AWDNS_CLIENT_SECRET", "AWDNS_BASE_URL", "AWDNS_TEST_FQDN"} {
		if os.Getenv(k) == "" {
			t.Skipf("%s not set; skipping awdns acceptance test", k)
		}
	}
}

func testAccProviderConfig() string {
	return fmt.Sprintf(`
provider "awdns" {
  client_id     = %q
  client_secret = %q
  base_url      = %q
}
`, os.Getenv("AWDNS_CLIENT_ID"), os.Getenv("AWDNS_CLIENT_SECRET"), os.Getenv("AWDNS_BASE_URL"))
}

// TestAccDomainDataSource_byFQDN looks up a known domain by fqdn and checks the
// computed id/name are populated.
func TestAccDomainDataSource_byFQDN(t *testing.T) {
	fqdn := os.Getenv("AWDNS_TEST_FQDN")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig() + fmt.Sprintf(`
data "awdns_domain" "test" {
  fqdn = %q
}
`, fqdn),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.awdns_domain.test", "fqdn", fqdn),
					resource.TestCheckResourceAttrSet("data.awdns_domain.test", "id"),
					resource.TestCheckResourceAttrSet("data.awdns_domain.test", "name"),
				),
			},
		},
	})
}

// TestAccDomainsDataSource_list lists all domains and checks the computed
// collection is present.
func TestAccDomainsDataSource_list(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig() + `
data "awdns_domains" "all" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.awdns_domains.all", "domains.#"),
				),
			},
		},
	})
}

// TestAccDomainsDataSource_filtered lists domains narrowed by dns_provider.
func TestAccDomainsDataSource_filtered(t *testing.T) {
	provider := os.Getenv("AWDNS_TEST_DNS_PROVIDER")
	if provider == "" {
		t.Skip("AWDNS_TEST_DNS_PROVIDER not set; skipping filtered list test")
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig() + fmt.Sprintf(`
data "awdns_domains" "filtered" {
  filter = {
    dns_provider = %q
  }
}
`, provider),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.awdns_domains.filtered", "domains.#"),
				),
			},
		},
	})
}
