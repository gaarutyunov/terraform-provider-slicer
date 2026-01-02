data "slicer_secret" "example" {
  name = "example-secret"
}

output "secret_size" {
  value = data.slicer_secret.example.size
}
