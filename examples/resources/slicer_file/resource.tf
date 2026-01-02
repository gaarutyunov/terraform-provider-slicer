resource "slicer_file" "example" {
  hostname    = "w1-medium-1"
  destination = "/tmp/example.txt"
  content     = "Hello, World!"
  permissions = "0644"
}
