# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-06-02

Initial release: the provider scaffold on terraform-plugin-framework (protocol
6), wired to the `awdns-go` SDK over the OAuth2 client-credentials grant.

### Added

- Provider scaffold built on terraform-plugin-framework (Terraform plugin
  protocol 6), serving `registry.terraform.io/AuthenticWeb/awdns` with a
  `-debug` flag (`main.go`).
- Provider configuration schema: `client_id`, `client_secret` (sensitive),
  `base_url`, and `request_timeout`.
- OAuth2 client-credentials authentication: `Configure` builds the `awdns-go`
  SDK client and exposes it to resources and data sources.
- Environment-variable fallback for credentials — `AWDNS_CLIENT_ID`,
  `AWDNS_CLIENT_SECRET`, and `AWDNS_BASE_URL`. A provider argument takes
  precedence; the environment variable is the fallback, so credentials can be
  kept out of `.tf` files.
- `awdns_domain` data source: look up a single domain by `id` or `fqdn`.
- `awdns_domains` data source: list domains with an optional client-side
  `filter` (by `status` and/or `dns_provider`).
- `awdns_dns_record` resource: full CRUD plus `terraform import` by
  `domain_id/record_id`, targeting the projected record API surface.
- Generated reference documentation under `docs/` (via `tfplugindocs`) and
  runnable examples under `examples/`.
- Packaging scaffolding: `terraform-registry-manifest.json` and `.goreleaser.yml`.

### Known limitations

- The `awdns_dns_record` resource targets the *projected* record API; the
  upstream record endpoints are not yet deployed.
- `base_url` carries the `/external/v1` suffix while the SDK appends its own base
  path — reconcile before first live use (see README, "Known scaffold caveats").
- `request_timeout` is accepted in the schema but not yet enforced by the SDK.

[0.1.0]: https://github.com/AuthenticWeb/terraform-provider-awdns/releases/tag/v0.1.0
