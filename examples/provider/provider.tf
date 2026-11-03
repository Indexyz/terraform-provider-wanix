terraform {
  required_providers {
    wanix = {
      source = "Indexyz/wanix"
    }
  }
}

provider "wanix" {
  config_path = pathexpand("~/.kube/config")
}
