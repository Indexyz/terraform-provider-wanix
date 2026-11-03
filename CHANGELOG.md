## 0.1.1

BUG FIXES:

- Fix code generation after upgrading copywrite to v0.25.3 by replacing the stale Koanf v1 dependency with compatible split modules and Koanf v2.

## 0.1.0

FEATURES:

- Add `wanix_instance` for `pop.wanix.net/v1alpha1` Instance custom resources.
- Update Instance specs using Kubernetes PUT with resourceVersion conflict retries, without PATCH or server-side apply.
- Support interfaces, routes, tunnels, WireGuard, firewall rules, workload images, and agent environment variables from sdwan-operator.
- Support Kubernetes authentication through kubeconfig files, base64-encoded kubeconfig, explicit credentials, and in-cluster service accounts.
- Expose reconciliation status and Pod IPs, support namespace/name imports, and wait for readiness and deletion.
- Document migration from the backend's Kubernetes provider resources.
