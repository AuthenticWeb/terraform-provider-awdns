terraform {
  required_providers {
    awdns = {
      source = "AuthenticWeb/awdns"
    }
  }
}

variable "awdns_client_id" {
  type = string
}

variable "awdns_client_secret" {
  type      = string
  sensitive = true
}

provider "awdns" {
  client_id     = var.awdns_client_id
  client_secret = var.awdns_client_secret
}

# List every domain.
data "awdns_domains" "all" {}

# List only active (status 7) domains served by the "ns1" DNS provider. The
# filter is applied client-side; omit a field to leave that dimension open.
data "awdns_domains" "active_ns1" {
  filter = {
    status       = 7
    dns_provider = "ns1"
  }
}

output "all_fqdns" {
  value = [for d in data.awdns_domains.all.domains : d.fqdn]
}

output "active_ns1_count" {
  value = length(data.awdns_domains.active_ns1.domains)
}
