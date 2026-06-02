terraform {
  required_providers {
    awdns = {
      source = "AuthenticWeb/awdns"
    }
  }
}

# Authenticate with an OAuth2 client-credentials pair issued by Authentic Web.
provider "awdns" {
  client_id     = var.awdns_client_id
  client_secret = var.awdns_client_secret

  # Optional overrides:
  # base_url        = "https://api.authenticweb.com/external/v1"
  # request_timeout = 30
}

variable "awdns_client_id" {
  type        = string
  description = "OAuth2 client ID for the AW external API."
}

variable "awdns_client_secret" {
  type        = string
  description = "OAuth2 client secret for the AW external API."
  sensitive   = true
}
