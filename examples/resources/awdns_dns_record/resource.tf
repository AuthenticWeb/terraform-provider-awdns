# NOTE: awdns_dns_record is part of the PROJECTED provider surface. It is not
# implemented in this scaffold — the AW external API record write endpoints are
# not yet available — and will land in a later component. This file documents
# the intended shape (resource prefix `awdns_`, per the Component 4/6 decision).

resource "awdns_dns_record" "example" {
  zone  = "example.com"
  name  = "www"
  type  = "A"
  value = "203.0.113.10"
  ttl   = 3600
}
