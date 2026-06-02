variable "awdns_client_id" {
  type        = string
  description = "OAuth2 client ID for the AW external API (client-credentials grant). May also be set via the AWDNS_CLIENT_ID environment variable; the argument takes precedence."
}

variable "awdns_client_secret" {
  type        = string
  description = "OAuth2 client secret for the AW external API. May also be set via the AWDNS_CLIENT_SECRET environment variable; the argument takes precedence."
  sensitive   = true
}

variable "awdns_base_url" {
  type        = string
  description = "Base URL of the AW external API. Override only for non-production environments."
  default     = "https://api.authenticweb.com/external/v1"
}

variable "domain_fqdn" {
  type        = string
  description = "Fully-qualified name of the existing AW-hosted zone to manage records in."
}
