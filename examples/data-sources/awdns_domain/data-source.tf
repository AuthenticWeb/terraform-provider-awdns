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

# Look up a single domain by its fully-qualified name...
data "awdns_domain" "by_fqdn" {
  fqdn = "example.com"
}

# ...or by its numeric id (supply exactly one of id / fqdn).
data "awdns_domain" "by_id" {
  id = "42"
}

output "example_status" {
  value = data.awdns_domain.by_fqdn.status
}

output "example_nameservers" {
  value = data.awdns_domain.by_fqdn.nameserver_list
}
