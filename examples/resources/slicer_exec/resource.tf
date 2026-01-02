resource "slicer_exec" "example" {
  hostname = "w1-medium-1"
  command  = "echo"
  args     = ["Hello, World!"]

  triggers = {
    always_run = timestamp()
  }
}

output "stdout" {
  value = slicer_exec.example.stdout
}
