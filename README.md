# Terraform Provider for Slicer

This provider allows you to manage [Slicer](https://slicervm.dev) VMs and related resources using Terraform.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.23 (for building from source)

## Installation

```hcl
terraform {
  required_providers {
    slicer = {
      source  = "gaarutyunov/slicer"
      version = "~> 0.1"
    }
  }
}
```

## Configuration

```hcl
provider "slicer" {
  endpoint = "https://slicer.example.com"
  token    = var.slicer_token

  # Optional
  timeout  = "60s"
  insecure = false
}
```

### Environment Variables

- `SLICER_ENDPOINT` - The Slicer API endpoint URL
- `SLICER_TOKEN` - The bearer token for authentication

## Resources

### `slicer_vm`

Manages a Slicer VM.

```hcl
resource "slicer_vm" "example" {
  host_group = "w1-medium"

  # Optional
  cpus       = 2
  ram_gb     = 8
  persistent = false

  import_user = "github-username"
  ssh_keys    = ["ssh-ed25519 AAAA..."]

  userdata = file("${path.module}/userdata.sh")

  tags = {
    role    = "k3s-control-plane"
    cluster = "platform"
  }

  secrets = ["db-password"]
}

output "vm_ip" {
  value = slicer_vm.example.ip
}
```

### `slicer_exec`

Executes a command on a Slicer VM.

```hcl
resource "slicer_exec" "install" {
  hostname = slicer_vm.example.hostname
  command  = "curl -sfL https://get.k3s.io | sh -"

  triggers = {
    vm_id = slicer_vm.example.id
  }
}

output "exit_code" {
  value = slicer_exec.install.exit_code
}
```

### `slicer_file`

Copies a file to a Slicer VM.

```hcl
resource "slicer_file" "config" {
  hostname    = slicer_vm.example.hostname
  destination = "/etc/app/config.yaml"
  content     = yamlencode(var.config)
  permissions = "0644"
}
```

### `slicer_secret`

Manages a Slicer secret.

```hcl
resource "slicer_secret" "db_password" {
  name  = "db-password"
  value = var.db_password
}
```

## Data Sources

### `data.slicer_vm`

Fetches information about an existing VM.

```hcl
data "slicer_vm" "existing" {
  hostname = "w1-medium-1"
}

output "vm_ip" {
  value = data.slicer_vm.existing.ip
}
```

### `data.slicer_vms`

Lists VMs with optional filtering.

```hcl
data "slicer_vms" "k3s_nodes" {
  filter {
    tag = "role=k3s-control-plane"
  }
}

output "k3s_ips" {
  value = [for vm in data.slicer_vms.k3s_nodes.vms : vm.ip]
}
```

### `data.slicer_hostgroups`

Lists available host groups.

```hcl
data "slicer_hostgroups" "available" {}

output "host_groups" {
  value = data.slicer_hostgroups.available.names
}
```

### `data.slicer_secret`

Fetches metadata about a secret.

```hcl
data "slicer_secret" "existing" {
  name = "api-key"
}

output "secret_size" {
  value = data.slicer_secret.existing.size
}
```

## Development

### Building

```bash
go build -o terraform-provider-slicer
```

### Testing

```bash
make testacc
```

## License

MPL-2.0
