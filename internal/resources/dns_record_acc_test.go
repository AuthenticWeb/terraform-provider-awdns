// Acceptance tests for the awdns_dns_record resource, exercised through a real
// in-process provider server with resource.Test(). They live in the external
// resources_test package so they can import internal/provider without a cycle.
//
// resource.Test() is a no-op unless TF_ACC is set, so `go test ./...` never
// touches the network. testAccPreCheck additionally skips unless live AWDNS_*
// credentials AND a target domain id are present. These target the PROJECTED
// record API (create/read/update/delete + import) and therefore stay skipped
// until the upstream record endpoints exist — at which point they assert the
// full resource lifecycle end-to-end against the real service.
package resources_test

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/AuthenticWeb/terraform-provider-awdns/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"awdns": providerserver.NewProtocol6WithError(provider.New()),
}

func testAccPreCheck(t *testing.T) {
	for _, k := range []string{"AWDNS_CLIENT_ID", "AWDNS_CLIENT_SECRET", "AWDNS_BASE_URL", "AWDNS_TEST_DOMAIN_ID"} {
		if os.Getenv(k) == "" {
			t.Skipf("%s not set; skipping awdns_dns_record acceptance test", k)
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

// TestAccDNSRecordResource_lifecycle creates a record, updates a non-replacing
// attribute (ttl), then imports it — covering create/read/update/import.
func TestAccDNSRecordResource_lifecycle(t *testing.T) {
	domainID := os.Getenv("AWDNS_TEST_DOMAIN_ID")
	config := func(ttl int) string {
		return testAccProviderConfig() + fmt.Sprintf(`
resource "awdns_dns_record" "test" {
  domain_id = %q
  subdomain = "tf-acc-test"
  type      = "A"
  ttl       = %d
  values    = ["203.0.113.10"]
}
`, domainID, ttl)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // Create + Read
				Config: config(3600),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("awdns_dns_record.test", "domain_id", domainID),
					resource.TestCheckResourceAttr("awdns_dns_record.test", "subdomain", "tf-acc-test"),
					resource.TestCheckResourceAttr("awdns_dns_record.test", "type", "A"),
					resource.TestCheckResourceAttr("awdns_dns_record.test", "ttl", "3600"),
					resource.TestCheckResourceAttr("awdns_dns_record.test", "values.#", "1"),
					resource.TestCheckResourceAttrSet("awdns_dns_record.test", "id"),
				),
			},
			{ // Update in place (ttl is not RequiresReplace)
				Config: config(7200),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("awdns_dns_record.test", "ttl", "7200"),
				),
			},
			{ // Import by "domain_id/record_id"
				ResourceName:      "awdns_dns_record.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["awdns_dns_record.test"]
					if rs == nil {
						return "", fmt.Errorf("awdns_dns_record.test not found in state")
					}
					return fmt.Sprintf("%s/%s", rs.Primary.Attributes["domain_id"], rs.Primary.Attributes["id"]), nil
				},
			},
		},
	})
}

// TestAccDNSRecordResource_invalidType asserts the plan-time type validator
// rejects an unsupported record type before any API call.
func TestAccDNSRecordResource_invalidType(t *testing.T) {
	domainID := os.Getenv("AWDNS_TEST_DOMAIN_ID")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig() + fmt.Sprintf(`
resource "awdns_dns_record" "bad" {
  domain_id = %q
  subdomain = "tf-acc-test"
  type      = "BOGUS"
  values    = ["203.0.113.10"]
}
`, domainID),
				ExpectError: regexp.MustCompile("Invalid DNS record type"),
			},
		},
	})
}
