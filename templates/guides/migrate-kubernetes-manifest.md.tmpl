# 从 kubernetes_manifest 迁移到 wanix_instance

新资源管理 `pop.wanix.net/v1alpha1/Instance` 实例，不是 CRD 定义本身。集群仍需预先安装 sdwan-operator 和 `instances.pop.wanix.net` CRD。

## 连接 Kubernetes

```hcl
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
```

也可以直接传入 base64 编码的 kubeconfig，不必落盘：

```hcl
variable "kubeconfig_base64" {
  type      = string
  sensitive = true
}

provider "wanix" {
  kubeconfig_base64 = var.kubeconfig_base64
  # config_context = "my-context"
}
```

`kubeconfig_base64` 标记为 sensitive，可与 `config_context` 一起使用，但不能与 `config_path`、`host`、`in_cluster_config` 混用。建议 kubeconfig 内嵌证书和密钥数据，不引用本地文件。Base64 并非加密，仍需按凭证保护。

也可在 runner Pod 内使用 `in_cluster_config = true`。显式连接时指定 `host`、`token` 或客户端证书，以及 `cluster_ca_certificate`，不要同时配置 kubeconfig。

## 替换 backend 模板

保留 `relayd-kubernetes` / `relayd-tke` 原有的地址分配、环境变量和镜像版本查询逻辑，将 `local.kube_spec` 转换为 snake_case 字段名。`agent.env`、labels、annotations 和接口名称等 **map key 不会被改写**。

| 旧 CRD 字段 | 新 Terraform spec 字段 |
| --- | --- |
| disableWorkload | disable_workload |
| workloadKind | workload_kind |
| imagePullSecrets | image_pull_secrets |
| podTemplateLabels | pod_template_labels |
| podTemplateAnnotations | pod_template_annotations |
| images.networkSetup | images.network_setup |
| interfaces.*.useGateway | interfaces.*.use_gateway |
| interfaces.*.virtualDevice | interfaces.*.virtual_device |
| firewall.masqueradeInterfaces | firewall.masquerade_interfaces |
| firewall.portForwarding | firewall.port_forwarding |
| tunnels.*.wireguard.privateKey | tunnels.*.wireguard.private_key |
| tunnels.*.wireguard.peers.*.publicKey | tunnels.*.wireguard.peers.*.public_key |
| tunnels.*.wireguard.peers.*.allowedIPs | tunnels.*.wireguard.peers.*.allowed_ips |
| tunnels.*.wireguard.peers.*.persistentKeepalive | tunnels.*.wireguard.peers.*.persistent_keepalive |

HTTP 镜像查询返回的对象也需显式映射：

```hcl
images = {
  agent         = try(local.version_data.images.agent, null)
  core          = try(local.version_data.images.core, null)
  network_setup = try(local.version_data.images.networkSetup, null)
  pause         = try(local.version_data.images.pause, null)
}
```

替换 `kubernetes_manifest.outbound`：

```hcl
resource "wanix_instance" "outbound" {
  count = data.manidae_instance.current.start_count == 0 ? 0 : 1

  name      = lower(data.manidae_parameter.name.value)
  namespace = var.kubernetes_namespace
  labels = {
    "sdwan.wanix.net/version" = data.manidae_parameter.version.value
    "sdwan.wanix.net/id"      = tostring(data.manidae_instance.current.id)
  }

  # local.kube_spec 需先按上表转换为 snake_case。
  spec = local.kube_spec
}

locals {
  instanceIp = try(wanix_instance.outbound[0].pod_ips[0], null)
}
```

然后删除：

- `null_resource.kube_update` 和对应 `replace_triggered_by`：spec 变更现在原地更新 Instance，不必删除重建。
- `computed_fields`：不再依赖 manifest 动态类型推断。
- `data.kubernetes_resource.outbound`：改为直接读取新资源的 `pod_ips`。

原模板若不使用 `count`，继续省略。`tencentcloud-kubernets` 模板中的 `cpu = 2` 推荐改为 `cpu = "2"`。

该模板的 `interfaces.tunnel.mtu` **不在当前 operator 的 InterfaceConfig/CRD 定义内**，应去掉；`mtu` 仅支持放在 tunnel 或 wireguard 对象中。

## 迁移现有 state，避免误删在线实例

1. 备份 state，暂停并发 apply。
2. 把配置改为新资源，但**不要直接 apply**。
3. 仅移除旧地址的 state，不删除 Kubernetes 对象：

   ```sh
   terraform state rm 'kubernetes_manifest.outbound[0]'
   ```

4. 按实际 namespace/name 导入已有对象：

   ```sh
   terraform import 'wanix_instance.outbound[0]' '<namespace>/<name>'
   ```

5. 执行 `terraform plan`，确认没有意外删除或替换后再 apply。两种 provider 的 state schema 不同，不能用 `terraform state mv` 直接迁移。

## 更新和等待语义

- Create 使用 POST；Update 使用 GET + PUT，遇到 resourceVersion 冲突会重试；不发 PATCH/SSA。
- Terraform 独占管理整个 `spec`。移除字段、列表项或接口后，PUT 会移除旧配置，不与旧 spec 合并。
- 更新保留实时对象的 finalizers、ownerReferences、status 等服务端字段。labels/annotations 仅移除先前由本资源管理的 key，保留 operator 的额外 key。导入时已有 labels/annotations 会纳入管理，请检查首次 plan。
- 默认等待 `status.phase == Ready` 且 `status.podIPs` 非空。`spec.pause = true` 时不等待 Pod IP。`wait_for_ready = false` 可关闭创建/更新等待；删除仍等待 finalizer 完成。`wait_timeout_seconds` 默认 600 秒。
- operator 当前没有 `observedGeneration`，其 Ready 信号**不保证更新后的 Pod 已完成 rollout 或应用健康**；本资源不提供额外的 rollout 保证。
- 创建成功但等待失败时，资源 ID 会先保存到 state，避免留下未跟踪对象。
- WireGuard 私钥虽标记 sensitive，仍写入 Terraform state，必须保护好 state/backend。

Kubernetes 凭证至少需要 `pop.wanix.net` API group 下 `instances` 的 `get/create/update/delete` 权限，不需要修改 CRD 定义。
