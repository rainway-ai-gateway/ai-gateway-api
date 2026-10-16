# /epp-pool

## 1. 数据模型

**实例 id 约定（部署形态）**：EPP 以 StatefulSet 部署，实例 id = **Pod hostname**（EPP 的 `-instance-id` 缺省值即 hostname，同组副本共享完全相同的启动参数，无需按实例差异化配置）；`/epp-pool` 登记时实例 id 取 pod 名（如 `epp-0`/`epp-1`），部署流程 reconcile 池时从 StatefulSet pod 名生成 PATCH 内容。前提：StatefulSet 保证 hostname 稳定唯一（不可用随机名的 Deployment）；非 K8s 部署显式传 `-instance-id`。

```json
{
    "name": "EPP.pool",
    "groups": [
        {
            "name": "g1",
            "instances": [
                { "id": "epp-a", "host": "10.0.0.1", "port": 9002 }
            ]
        }
    ]
}
```

**字段说明**

| 字段 | 类型 | 说明 | 可能取值 | 合法性条件 |
|------|------|------|----------|----------|
| `name` | string | 实例池完整名称 | 如 `EPP.pool` | 请求中无需传入，由配置项 `RunTime.DefaultEPPInstancePoolName` 提供（默认值 `EPP.pool`） |
| `groups` | []Group | 实例组列表 | - | 必填；至少1个元素；组名非空、唯一 |

**Group 结构（`groups` 元素）**

| 字段 | 类型 | 说明 | 可能取值 | 合法性条件 |
|------|------|------|----------|----------|
| `name` | string | 实例组名 | 如 `g1` | 必填；非空；池内唯一 |
| `instances` | []Instance | 组内实例列表 | - | 必填；至少1个元素（拒绝空组） |

**Instance 结构（`groups[].instances` 元素）**

| 字段 | 类型 | 说明 | 可能取值 | 合法性条件 |
|------|------|------|----------|----------|
| `id` | string | 实例 id（EPP 以 `-instance-id` 启动参数对应此值） | 如 `epp-a` | 必填；非空；**池内全局唯一** |
| `host` | string | 实例主机名或 IP（IPv6 字面量不带括号） | 无 DNS 时可填写 IP 地址 | 必填；非空；类型为 [Hostname](./00-common.md#公共参数类型) 或 IP |
| `port` | int | 实例端口 | 如 `9002` | 必填；类型为 [Port](./00-common.md#公共参数类型) |

**约束**

- 实例池名称由配置项 `RunTime.DefaultEPPInstancePoolName` 提供（默认值 `EPP.pool`），请求中无需传入 `name`。
- `groups` 至少包含1个元素；组名非空、唯一。
- `groups[].instances` 至少包含1个元素（拒绝空组）。
- 每个实例的 `id` 非空，且池内全局唯一。
- 每个实例的 `host` 非空，类型为 [Hostname](./00-common.md#公共参数类型) 或 IP（IPv6 字面量不带括号）。
- 每个实例的 `port` 类型为 [Port](./00-common.md#公共参数类型)。
- `(host, port)` 组合池内全局唯一（等价于旧 host:port 唯一语义）。
- 每组实例数：恰好 1 个；拒绝空组与 2 个及以上实例的组（API 写入面不再接受主备双实例组）。
- 实例无 `status`/`last_heartbeat` 字段——存活感知在 BFE 侧（EPPAddr 连接滞回），api 侧不做存活标记。

## 2. 接口清单

### 2.1 获取 EPP 实例池详情

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 获取 EPP 实例池详情（实例组 + 实例列表） | - |
| 端点 | /epp-pool | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（Query）**

无。

**执行逻辑**

1. 从配置文件 `RunTime` 中读取 `DefaultEPPInstancePoolName`（默认值：`EPP.pool`）。
2. 查询 EPP 实例池，若不存在则返回错误。
3. 返回实例池详情（包含实例组 + 实例列表）。

**返回数据（Data内容）**

字段同 [1. 数据模型](#1-数据模型)。

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "name": "EPP.pool",
        "groups": [
            {
                "name": "g1",
                "instances": [
                    { "id": "epp-a", "host": "10.0.0.1", "port": 9002 }
                ]
            }
        ]
    }
}
```

### 2.2 全量替换 EPP 实例池

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 全量替换 EPP 实例池（实例组 + 实例列表） | 该更新是全量替换语义，不支持仅添加部分数据；部署流程在实例变更（扩缩容、换机）后调用 |
| 端点 | /epp-pool | - |
| 版本 | v1 | - |
| method | PATCH | - |

**输入参数（Body）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| groups | []Group | 实例组列表 | Y | 全量替换当前实例池中的实例组与实例 | 必填；数组至少1个元素；组名非空、唯一 |
| groups[].name | string | 实例组名 | Y | - | 必填；非空；池内唯一 |
| groups[].instances | []Instance | 组内实例列表 | Y | 拒绝空组 | 必填；数组至少1个元素 |
| groups[].instances[].id | string | 实例 id | Y | EPP 以 `-instance-id` 启动参数对应此值 | 必填；非空；池内全局唯一 |
| groups[].instances[].host | string | 实例主机名或 IP | Y | IPv6 字面量不带括号 | 必填；非空；类型为 [Hostname](./00-common.md#公共参数类型) 或 IP |
| groups[].instances[].port | int | 实例端口 | Y | - | 必填；类型为 [Port](./00-common.md#公共参数类型) |

**约束**

- 实例池名称由配置项 `RunTime.DefaultEPPInstancePoolName` 提供（默认值 `EPP.pool`），请求中无需传入 `name`。
- `groups` 至少包含1个元素；组名非空、唯一。
- `groups[].instances` 至少包含1个元素（拒绝空组）。
- 每个实例的 `id` 非空，且池内全局唯一。
- 每个实例的 `host` 非空，类型为 [Hostname](./00-common.md#公共参数类型) 或 IP（IPv6 字面量不带括号）。
- 每个实例的 `port` 类型为 [Port](./00-common.md#公共参数类型)。
- `(host, port)` 组合池内全局唯一（等价于旧 host:port 唯一语义）。
- 每组实例数：恰好 1 个；拒绝空组与 2 个及以上实例的组（API 写入面不再接受主备双实例组）。

**HTTP BODY参数示例**

```json
{
    "groups": [
        {
            "name": "g1",
            "instances": [
                { "id": "epp-a", "host": "10.0.0.1", "port": 9002 }
            ]
        },
        {
            "name": "g2",
            "instances": [
                { "id": "epp-c", "host": "10.0.0.3", "port": 9002 }
            ]
        }
    ]
}
```

**执行逻辑**

1. 校验请求参数合法性（组名非空唯一、实例 id 池内全局唯一、`(host, port)` 池内全局唯一、host/port 格式、组规模）。
2. 使用配置项 `DefaultEPPInstancePoolName` 定位 EPP 实例池。
3. 全量替换实例池（`epp_instances` 表，实例组 + 实例列表）。
4. 触发分配悬空自动修复：`/epp-pool` PATCH 后分配悬空（primary 实例已被移出池）时自动修复——① 组仍存在：同组剩余实例中重选 primary（不换组）；② 组已不存在：**跨组重分配**——以该 cluster 为目标对整个实例池重跑分配器选新组；池无可分配候选组时，清除该分配（cluster 进入未分配态，导出 server_data_conf 时降级为 `WRR` 并输出 error 日志，待容量恢复后自动修复路径重新分配）。
5. 返回更新后的实例池详情。

> **注意**：实例池变更不直接 bump `ConfigTopicEppData` topic（实例增减不改变 cluster→role 映射）；实例地址在生成 EPPAddr 时以 `net.JoinHostPort(host, strconv.Itoa(port))` 拼接为 `host:port`（IPv6 自动加括号），BFE 消费格式不变。

**返回数据（Data内容）**

同 [2.1 获取 EPP 实例池详情](#21-获取-epp-实例池详情)。

**成功返回示例**

同 [2.1 获取 EPP 实例池详情](#21-获取-epp-实例池详情)。

---
