# terraform-provider-awdns

Terraform provider for **Authentic Web DNS** — manage hosted zones and DNS
records as plain Terraform configuration, backed by the AW external API. It is
built on
[terraform-plugin-framework](https://github.com/hashicorp/terraform-plugin-framework)
(Terraform plugin protocol 6) and the
[`awdns-go`](https://github.com/AuthenticWeb/awdns-go) SDK, which authenticates
with the OAuth2 client-credentials grant.

**Who it's for:** Authentic Web customers and internal teams who want to read
their AW-hosted domains and declare DNS records in version-controlled Terraform
instead of clicking through a dashboard or calling the API by hand.

It currently ships:

| Type | Name | Purpose |
|---|---|---|
| Data source | [`awdns_domain`](docs/data-sources/domain.md) | Look up one domain by `id` or `fqdn`. |
| Data source | [`awdns_domains`](docs/data-sources/domains.md) | List domains, with an optional client-side `filter`. |
| Resource | [`awdns_dns_record`](docs/resources/dns_record.md) | Manage a DNS record within a domain. |

> **Status: scaffold.** Provider configuration, OAuth2 auth, and the domain data
> sources target the *live* domain endpoints. The `awdns_dns_record` resource is
> implemented and unit-tested but targets the *projected* record API surface — it
> applies cleanly once the upstream record endpoints are deployed. See
> [Known scaffold caveats](#known-scaffold-caveats).

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.5
- [Go](https://go.dev/dl/) >= 1.21 — only needed to build the provider from
  source. (This repo's `go.mod` pins the toolchain to `1.25.8`; with the default
  `GOTOOLCHAIN=auto`, `go build` fetches that toolchain automatically.)

## Installation

Declare the provider in your Terraform configuration's `required_providers`
block. Terraform installs it from the registry source `AuthenticWeb/awdns`:

```terraform
terraform {
  required_providers {
    awdns = {
      source  = "AuthenticWeb/awdns"
      version = "~> 0.1"
    }
  }
}
```

> Not yet published to the Terraform Registry. Until then, build and run it
> locally with `dev_overrides` — see [Local development](#local-development).

## Authentication

The provider authenticates to the AW external API with an OAuth2
**client-credentials** pair issued from your Authentic Web account. Provide it
either as provider arguments or via environment variables. **The argument takes
precedence; the environment variable is the fallback**, so credentials can be
kept out of `.tf` files entirely.

| Argument | Environment variable | Required | Default | Notes |
|---|---|---|---|---|
| `client_id` | `AWDNS_CLIENT_ID` | yes | — | OAuth2 client ID (client-credentials grant). |
| `client_secret` | `AWDNS_CLIENT_SECRET` | yes | — | OAuth2 client secret. Marked sensitive. |
| `base_url` | `AWDNS_BASE_URL` | no | `https://api.authenticweb.com/external/v1` | AW external API base URL. |
| `request_timeout` | — | no | `30` | Per-request timeout (seconds). Reserved; not yet enforced. |

`client_id` and `client_secret` are *required credentials*: each must be supplied
through its argument **or** its environment variable. If neither path provides a
value, the provider fails configuration with an error naming the missing
argument and its `AWDNS_*` variable.

**Recommended — keep secrets in the environment:**

```sh
export AWDNS_CLIENT_ID="your-client-id"
export AWDNS_CLIENT_SECRET="your-client-secret"
```

```terraform
# No credentials in the config; the provider reads them from AWDNS_* env vars.
provider "awdns" {}
```

**Or pass them as arguments (e.g. wired to variables):**

```terraform
provider "awdns" {
  client_id     = var.awdns_client_id
  client_secret = var.awdns_client_secret
}
```

## Quick start

Configure the provider, look up a domain, and manage a record under it:

```terraform
terraform {
  required_providers {
    awdns = {
      source  = "AuthenticWeb/awdns"
      version = "~> 0.1"
    }
  }
}

# Reads AWDNS_CLIENT_ID / AWDNS_CLIENT_SECRET from the environment.
provider "awdns" {}

# Look up an existing AW-hosted domain by its FQDN.
data "awdns_domain" "example" {
  fqdn = "example.com"
}

# Point www at a host. domain_id is wired straight from the data source.
resource "awdns_dns_record" "www" {
  domain_id = data.awdns_domain.example.id
  subdomain = "www"
  type      = "A"
  ttl       = 3600
  values    = ["203.0.113.10"]
  note      = "primary web frontend"
}

output "example_nameservers" {
  value = data.awdns_domain.example.nameserver_list
}
```

```sh
export AWDNS_CLIENT_ID="your-client-id"
export AWDNS_CLIENT_SECRET="your-client-secret"

terraform init
terraform plan
terraform apply
```

More examples live in [`examples/`](examples/).

## Documentation

Full, schema-accurate reference docs are generated into [`docs/`](docs/) and
render on the Terraform Registry:

- [Provider configuration](docs/index.md)
- Data sources: [`awdns_domain`](docs/data-sources/domain.md), [`awdns_domains`](docs/data-sources/domains.md)
- Resource: [`awdns_dns_record`](docs/resources/dns_record.md)

`docs/` is generated from the provider schema and `examples/` with
[`tfplugindocs`](https://github.com/hashicorp/terraform-plugin-docs); do not edit
the files by hand. Regenerate after any schema or example change:

```sh
go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest
tfplugindocs generate --provider-name awdns
```

## Local development

The provider builds against a local checkout of the `awdns-go` SDK via a
`replace` directive in `go.mod` (the SDK has no published `v0.1.0` tag yet):

```
replace github.com/AuthenticWeb/awdns-go => ../awdns-go
```

So `awdns-go` must sit beside this repo at `../awdns-go`.

### Build & test

```sh
go build ./...
go vet ./...
go test ./...          # unit tests; acceptance tests are skipped unless TF_ACC is set
```

Acceptance tests (`*_acc_test.go`) stand up the provider in-process and require a
live API plus credentials; they run only when `TF_ACC` and the documented
`AWDNS_ACC_*` variables are set.

### Try it with dev_overrides

Point Terraform at your locally built binary instead of the registry. Create a
`~/.terraformrc` (or set `TF_CLI_CONFIG_FILE`):

```hcl
provider_installation {
  dev_overrides {
    "AuthenticWeb/awdns" = "/absolute/path/to/your/GOBIN"
  }
  direct {}
}
```

Build the binary into that `GOBIN`, then in a config dir:

```sh
go install .                 # installs terraform-provider-awdns into $GOBIN
terraform providers schema -json | jq '.provider_schemas'
```

With `dev_overrides` active, `terraform init` is unnecessary (and warns); plan
and schema commands resolve the provider straight from the override path.

## Known scaffold caveats

- **`base_url` vs the SDK base path.** `awdns-go`'s `NewClient` expects the
  application *root* (e.g. `https://api.authenticweb.com`) and appends
  `/external/v1` itself, also hitting `/oauth/token` at the root. The provider's
  documented `base_url` default carries the `/external/v1` suffix, so passing it
  straight through would double the prefix at request time. This is latent today
  (the API is not live); reconcile — by either trimming in `Configure` or
  changing the default to the root — before first real use.
- **`request_timeout`** is accepted in the schema but not yet wired: the SDK
  hard-codes a 30s timeout and exposes no setter.
- **`awdns_dns_record` targets the projected record API.** The record endpoints
  are not deployed upstream yet; the resource is implemented and unit-tested so
  it works the moment they land.

## License

[MPL-2.0](./LICENSE).
