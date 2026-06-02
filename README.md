# terraform-provider-awdns

Terraform provider for **Authentic Web DNS** — manages hosted zones and DNS
records via the AW external API. Built on
[terraform-plugin-framework](https://github.com/hashicorp/terraform-plugin-framework)
(Terraform plugin protocol 6) and the
[`awdns-go`](https://github.com/AuthenticWeb/awdns-go) SDK, which authenticates
with the OAuth2 client-credentials grant.

> **Status: scaffold.** Provider configuration and the SDK client wiring are in
> place. Resources (e.g. `awdns_dns_record`) target the *projected* API surface
> and land in a later component once the upstream record endpoints exist.

## Provider configuration

| Argument | Type | Required | Default | Notes |
|---|---|---|---|---|
| `client_id` | string | yes | — | OAuth2 client ID (client-credentials grant). |
| `client_secret` | string (sensitive) | yes | — | OAuth2 client secret. |
| `base_url` | string | no | `https://api.authenticweb.com/external/v1` | AW external API base URL. |
| `request_timeout` | number | no | `30` | Per-request timeout (seconds). Reserved; not yet enforced. |

```terraform
terraform {
  required_providers {
    awdns = {
      source = "AuthenticWeb/awdns"
    }
  }
}

provider "awdns" {
  client_id     = var.awdns_client_id
  client_secret = var.awdns_client_secret
}
```

## Local development

The provider builds against a local checkout of the `awdns-go` SDK via a
`replace` directive in `go.mod` (the SDK has no published `v0.1.0` tag yet):

```
replace github.com/AuthenticWeb/awdns-go => ../awdns-go
```

So `awdns-go` must sit beside this repo (`../awdns-go`).

### Build & test

```bash
go build ./...
go test ./...
```

### Try it with dev_overrides

Point Terraform at your locally built binary instead of the Registry. Create a
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

```bash
go install .                 # installs terraform-provider-awdns into $GOBIN
terraform providers schema -json | jq '.provider_schemas'
```

With `dev_overrides` active, `terraform init` is unnecessary (and warns); plan
and schema commands resolve the provider straight from the override path.

### Documentation

`docs/` is generated from the schema and `examples/` with
[`tfplugindocs`](https://github.com/hashicorp/terraform-plugin-docs):

```bash
tfplugindocs generate --provider-name awdns
```

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

## License

[MPL-2.0](./LICENSE).
