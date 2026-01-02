data "slicer_vms" "k3s_nodes" {
  filter {
    tag = "role=k3s-control-plane"
  }
}

output "k3s_count" {
  value = data.slicer_vms.k3s_nodes.count
}

output "k3s_ips" {
  value = [for vm in data.slicer_vms.k3s_nodes.vms : vm.ip]
}
