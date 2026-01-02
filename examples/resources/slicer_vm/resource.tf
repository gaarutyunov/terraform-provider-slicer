resource "slicer_vm" "example" {
  host_group = "w1-medium"

  cpus       = 2
  ram_gb     = 8
  persistent = false

  import_user = "gaarutyunov"

  tags = {
    role        = "example"
    environment = "dev"
  }
}

output "vm_hostname" {
  value = slicer_vm.example.hostname
}

output "vm_ip" {
  value = slicer_vm.example.ip
}
