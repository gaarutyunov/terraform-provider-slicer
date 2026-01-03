terraform {
  required_providers {
    slicer = {
      source = "gaarutyunov/slicer"
    }
  }
}

provider "slicer" {
  endpoint = "https://slicer.garutyunov.com"
  token    = "test-token"
}

# Test data source - list host groups
data "slicer_hostgroups" "all" {}

output "hostgroups" {
  value = data.slicer_hostgroups.all.hostgroups
}
