data "slicer_hostgroups" "available" {}

output "hostgroup_names" {
  value = data.slicer_hostgroups.available.names
}
