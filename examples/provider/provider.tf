terraform {
  required_providers {
    slicer = {
      source = "gaarutyunov/slicer"
    }
  }
}

provider "slicer" {
  endpoint = "https://slicer.example.com"
  token    = var.slicer_token
}

variable "slicer_token" {
  type        = string
  sensitive   = true
  description = "Slicer API token"
}
