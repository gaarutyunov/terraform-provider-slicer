data "slicer_vm" "example" {
  hostname = "w1-medium-1"
}

output "vm_ip" {
  value = data.slicer_vm.example.ip
}
