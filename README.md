# Terraform Provider for TrueNAS

[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![GitHub release](https://img.shields.io/github/v/release/deevus/terraform-provider-truenas)](https://github.com/deevus/terraform-provider-truenas/releases)
[![Terraform Provider Downloads](https://img.shields.io/terraform/provider/dt/1361211)](https://registry.terraform.io/providers/deevus/truenas/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/deevus/terraform-provider-truenas)](go.mod)
[![Go Report Card](https://goreportcard.com/badge/github.com/deevus/terraform-provider-truenas)](https://goreportcard.com/report/github.com/deevus/terraform-provider-truenas)
[![Commercial Support](https://img.shields.io/badge/support-available-brightgreen)](#support)

A Terraform provider for managing TrueNAS SCALE and Community editions.

## Installation

```hcl
terraform {
  required_providers {
    truenas = {
      source  = "deevus/truenas"
      version = "~> 0.1"
    }
  }
}
```

## Build And Install

```bash
make build
make install
```

`make build` writes the provider binary to the repo root as `terraform-provider-truenas`.

`make install` copies it to:

`~/.terraform.d/plugins/registry.terraform.io/deevus/truenas/${VERSION}/${GOOS}_${GOARCH}/`

## Terraform CLI Config

For local provider development, copy [example.tfrc](example.tfrc) to `~/.terraformrc`.

The included example already points at:

`/Users/aurimas/Code/GitHub/yavasura/terraform-provider-truenas`

With that `dev_overrides` entry in place, any Terraform project that requests `deevus/truenas` will use the binary built in this repo root after you run `make build`.

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

Full documentation is available on the [Terraform Registry](https://registry.terraform.io/providers/deevus/truenas/latest/docs).

## Requirements

- TrueNAS SCALE or TrueNAS Community
- SSH access with a user configured for `midclt`, `rm`, and `rmdir` (see [User Setup](https://registry.terraform.io/providers/deevus/truenas/latest/docs#truenas-user-setup))

## Support

Need help deploying terraform-provider-truenas at your org? I offer implementation support, custom development, and training through my consultancy: [simonhartcher.com](https://simonhartcher.com). Email in bio.

## License

MIT License - see [LICENSE](LICENSE) for details.
