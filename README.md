# Terraform Provider for TrueNAS

[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

A Terraform provider for managing TrueNAS SCALE and Community editions.

This fork is independently identified as `yavasura/truenas`, with Go module
`github.com/yavasura/terraform-provider-truenas`. It originates from
[deevus/terraform-provider-truenas](https://github.com/deevus/terraform-provider-truenas)
and retains its GitHub fork relationship. The upstream MIT copyright and historical
changelog are preserved; the `github.com/deevus/truenas-go` dependency remains unchanged.

## Installation

```hcl
terraform {
  required_providers {
    truenas = {
      source  = "yavasura/truenas"
      version = "~> 0.1"
    }
  }
}
```

Registry installation requires a published `yavasura/truenas` release; publication
is not established by this identity change. Until then, use a local build below.
Choose a version constraint matching the releases actually available.

## Migrating from deevus/truenas

Back up your Terraform state securely and update `required_providers` to
`yavasura/truenas`. Once the new provider is available, run in each workspace:

```bash
terraform state replace-provider \
  registry.terraform.io/deevus/truenas \
  registry.terraform.io/yavasura/truenas
terraform init
terraform plan
```

Review the replacement confirmation and plan before applying. This changes the
provider address in state, not resource names or infrastructure. Do not rename
`truenas_*` resources or the local provider name `truenas`.

## Build And Install

```bash
make build
make install
```

`make build` writes the provider binary to the repo root as `terraform-provider-truenas`.

`make install` copies it to:

`~/.terraform.d/plugins/registry.terraform.io/yavasura/truenas/${VERSION}/${GOOS}_${GOARCH}/`

## Terraform CLI Config

For local provider development, copy [example.tfrc](example.tfrc) to `~/.terraformrc`.

Replace the placeholder path in the example with the absolute path to your checkout:

`/absolute/path/to/terraform-provider-truenas`

With that `dev_overrides` entry in place, any Terraform project that requests `yavasura/truenas` will use the binary built in this repo root after you run `make build`.

## Usage

```hcl
provider "truenas" {
  host        = "192.168.1.100"
  auth_method = "ssh"

  ssh {
    user                 = "terraform"
    private_key          = file("~/.ssh/terraform_ed25519")
    host_key_fingerprint = "SHA256:..."  # ssh-keyscan <host> | ssh-keygen -lf -
  }
}

# Create a dataset
resource "truenas_dataset" "example" {
  pool = "tank"
  name = "example"
}
```

## Features

- **Data Sources**: Query pools and datasets
- **Resources**: Manage datasets, host paths, files, and applications

## Documentation

Provider documentation is available in [docs/index.md](docs/index.md). Once published,
the registry address is [yavasura/truenas](https://registry.terraform.io/providers/yavasura/truenas/latest/docs).

## Requirements

- TrueNAS SCALE or TrueNAS Community
- SSH access with a user configured for `midclt`, `rm`, and `rmdir` (see [User Setup](docs/index.md#truenas-user-setup))

## Releases and signing

Releases target [yavasura/terraform-provider-truenas](https://github.com/yavasura/terraform-provider-truenas).
Release automation is disabled until the repository variable `RELEASE_ENABLED` is
set to `true`. Before enabling it:

1. Create a signing key controlled by the fork maintainer. The upstream public key
   is not this fork's signing identity and is not distributed here.
2. Add the private key as the GitHub Actions secret `GPG_PRIVATE_KEY` (never commit
   it). Export only its public key to `GPG_PUBLIC_KEY.asc`, record its fingerprint,
   and register that public key with the Terraform Registry for `yavasura/truenas`.
3. Confirm repository release permissions, registry namespace ownership, and the
   registry's GitHub repository integration. Validate the GoReleaser configuration
   and signed checksums before publishing a version tag.

The workflow derives `GPG_FINGERPRINT` from the imported key. No fork signing key,
registry registration, or published release is supplied by this identity change.

## License

MIT License - see [LICENSE](LICENSE) for details.
