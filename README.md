# Terraform Provider Wanix

This provider manages the `pop.wanix.net/v1alpha1` `Instance` custom resource from `sdwan-operator`.

`wanix_instance` deliberately avoids the Kubernetes provider's patch path: creates use `POST`, updates use conflict-retried full-object `PUT`, and deletes use `DELETE`. This is useful for the `Instance` CRD because operator-managed fields and CRD patch behavior can otherwise cause update failures.

## Requirements

- Terraform >= 1.0
- Go >= 1.25
- The `pop.wanix.net/v1alpha1` CRD and `sdwan-operator` installed in the target cluster

## Base64 kubeconfig

The provider can load a standard base64-encoded kubeconfig directly in memory, without writing a temporary file:

```hcl
variable "kubeconfig_base64" {
  type      = string
  sensitive = true
}

provider "wanix" {
  kubeconfig_base64 = var.kubeconfig_base64
  # config_context = "my-context" # Optional; defaults to current-context.
}
```

`kubeconfig_base64` cannot be combined with `config_path`, `host`, or `in_cluster_config`. Embed certificates and keys in the kubeconfig (`certificate-authority-data`, `client-certificate-data`, `client-key-data`) rather than referencing local files. Standard base64 encoding is not encryption; handle the value as a credential.

## Development

```shell
make test
make build
```

See [`docs/resources/instance.md`](docs/resources/instance.md) and [`examples/resources/wanix_instance/resource.tf`](examples/resources/wanix_instance/resource.tf) for usage. Existing backend templates can follow the [migration guide](docs/guides/migrate-kubernetes-manifest.md).

The Terraform lifecycle tests run against an in-process Kubernetes API stub, including conflict retries and a prohibition on PATCH. They do not require a real cluster. Live operator integration still needs to be verified against your cluster.

The examples use the provider address `Indexyz/wanix`; they do not imply that a release has been published. For local development, run `go install` and configure a Terraform CLI `dev_overrides` entry for `registry.terraform.io/Indexyz/wanix` pointing to your Go binary directory.
