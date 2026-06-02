# Manage a DNS record within an Authentic Web DNS domain.
#
# NOTE: the record endpoints target the PROJECTED API surface — upstream
# currently exposes only domain list/show. The resource is implemented and
# unit-tested; applying it against the live API works once the record endpoints
# are deployed.

# Look up the parent domain so we can wire its id straight into the record.
data "awdns_domain" "example" {
  fqdn = "example.com"
}

resource "awdns_dns_record" "www" {
  domain_id = data.awdns_domain.example.id
  subdomain = "www"
  type      = "A"
  ttl       = 3600
  values    = ["203.0.113.10"]
  note      = "primary web frontend"
}

# A multi-value record set: several values under one record.
resource "awdns_dns_record" "mail" {
  domain_id = data.awdns_domain.example.id
  subdomain = "@"
  type      = "MX"
  values    = ["10 mail1.example.com.", "20 mail2.example.com."]
}

# Import an existing record by "domain_id/record_id":
#   terraform import awdns_dns_record.www 42/1001
