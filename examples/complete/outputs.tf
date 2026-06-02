output "domain_id" {
  description = "Numeric ID of the managed zone, resolved from its FQDN."
  value       = data.awdns_domain.zone.id
}

output "domain_nameservers" {
  description = "Authoritative nameservers for the zone."
  value       = data.awdns_domain.zone.nameserver_list
}

output "domain_status" {
  description = "Upstream status code of the zone (e.g. 7 = active)."
  value       = data.awdns_domain.zone.status
}

output "record_ids" {
  description = "API-assigned record IDs, keyed by logical record name."
  value = {
    www  = awdns_dns_record.www.id
    blog = awdns_dns_record.blog.id
    mx   = awdns_dns_record.mx.id
    spf  = awdns_dns_record.spf.id
  }
}
