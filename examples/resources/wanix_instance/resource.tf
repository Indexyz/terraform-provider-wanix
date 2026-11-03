resource "wanix_instance" "outbound" {
  name      = "outbound"
  namespace = "default"

  labels = {
    "sdwan.wanix.net/id" = "123"
  }

  spec = {
    cpu              = "100m"
    memory           = 4096
    workload_kind    = "StatefulSet"
    disable_workload = false
    pause            = false

    interfaces = {
      region_a = {
        address = "192.0.2.10"
        gateway = "192.0.2.1"
        table   = 222
        default = true
        routes = [
          {
            destination = "198.51.100.0/24"
            gateway     = "192.0.2.1"
          }
        ]
      }
    }

    image_pull_secrets = ["registry-creds"]
    pod_template_annotations = {
      "example.wanix.net/restarted-at" = "2026-04-15T00:00:00Z"
    }
    agent = {
      env = {
        DISABLE_PUBLIC_LISTENER = "true"
      }
    }
  }

  wait_for_ready       = true
  wait_timeout_seconds = 600
}

output "instance_ip" {
  value = try(wanix_instance.outbound.pod_ips[0], null)
}
