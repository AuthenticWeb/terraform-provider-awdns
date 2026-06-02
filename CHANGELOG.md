# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Initial provider scaffold built on terraform-plugin-framework (protocol 6).
- Provider configuration schema: `client_id` (required), `client_secret`
  (required, sensitive), `base_url` (optional), `request_timeout` (optional).
- `Configure` constructs the `awdns-go` SDK client from the OAuth2
  client-credentials configuration and exposes it to resources/data sources.
- Provider server entry point (`main.go`) serving
  `registry.terraform.io/AuthenticWeb/awdns`, with a `-debug` flag.
- Packaging scaffolding: `terraform-registry-manifest.json`, `.goreleaser.yml`,
  `examples/`, and a `docs/` placeholder for `tfplugindocs`.
