# Complete awdns example: configure the provider, resolve an existing AW-hosted
# zone by FQDN, and manage a representative set of record types under it
# (A, CNAME, MX, TXT).
#
# NOTE: the awdns_dns_record resource targets the PROJECTED record API surface —
# upstream currently exposes only domain list/show. `terraform validate` and the
# data-source reads work today; a full apply of the records lands once the
# record endpoints are deployed.

terraform {
  required_version = ">= 1.5"
  required_providers {
    awdns = {
      source  = "AuthenticWeb/awdns"
      version = "~> 0.1"
    }
  }
}

# Credentials are wired from variables so this example is self-contained. They
# may equally be supplied via the AWDNS_CLIENT_ID / AWDNS_CLIENT_SECRET
# environment variables, in which case the two arguments below can be dropped.
provider "awdns" {
  client_id     = var.awdns_client_id
  client_secret = var.awdns_client_secret
  base_url      = var.awdns_base_url
}

# Resolve the target zone by its FQDN; its numeric id feeds every record's
# domain_id, so the records always attach to the right zone.
data "awdns_domain" "zone" {
  fqdn = var.domain_fqdn
}

# A — IPv4 address for the web frontend.
resource "awdns_dns_record" "www" {
  domain_id = data.awdns_domain.zone.id
  subdomain = "www"
  type      = "A"
  ttl       = 300
  values    = ["203.0.113.10"]
  note      = "primary web frontend"
}

# CNAME — alias blog.<zone> to the www host. CNAME rdata is a single hostname.
resource "awdns_dns_record" "blog" {
  domain_id = data.awdns_domain.zone.id
  subdomain = "blog"
  type      = "CNAME"
  values    = ["www.${var.domain_fqdn}."]
}

# MX — mail exchangers at the apex. Each value is "<priority> <host>"; a
# multi-value list expresses the whole record set in a single resource.
resource "awdns_dns_record" "mx" {
  domain_id = data.awdns_domain.zone.id
  subdomain = "@"
  type      = "MX"
  ttl       = 3600
  values = [
    "10 mail1.${var.domain_fqdn}.",
    "20 mail2.${var.domain_fqdn}.",
  ]
}

# TXT — SPF policy at the apex. TXT rdata is a free-form quoted string.
resource "awdns_dns_record" "spf" {
  domain_id = data.awdns_domain.zone.id
  subdomain = "@"
  type      = "TXT"
  values    = ["v=spf1 include:_spf.${var.domain_fqdn} -all"]
}
