# /epp-assignments

## 1. 概述

EPP 实例组与 cluster 之间的分配由**分配器自动生成**（触发时机与算法见 `design-docs/modifications/2026-09-08-epp-scheduling-integration/design-changes.md` §4.2：cluster 进入 EPP 模式时自动选组选主、`/epp-pool` 变更后自动修复悬空分配），本域仅提供**查询视图**与**手工覆写**（运维干预入口）；分配输入（实例组列表）来自 `/epp-pool`（见 [epp-pool.md](./epp-pool.md)）。

> **说明**：本接口的视图只暴露主实例（`primary`）。备实例（standby）的读时展开仍存在于模型层与 InnerAPI 导出（`epp_data`、`EPPAddr`），用于兼容存量双实例组数据，但不在 OpenAPI 中下发。

## 2. 接口清单

### 2.1 获取分配全量视图

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 分配全量视图（per-cluster 展开 + 未分配/空闲清单） | - |
| 端点 | /epp-assignments | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（Query）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| cluster | string | 按 cluster 过滤 | N | 只返回该 cluster 的分配条目；不传返回全部 EPP 模式 cluster | 须为已存在且 `balance_mode=EPP` 的 cluster 名；否则返回 404 语义错误 |

**执行逻辑**

1. 查询全部（或指定）`balance_mode=EPP` 的 cluster 的分配记录。
2. 与 EPP 实例池（`/epp-pool`）做读时 join，展开 `primary` 实例详情。
3. 不一致时（如 primary 指向的实例已被从池中移除，或组已不存在）该条目标记 `degraded`：`primary` 返回 `null` 并计入 `unassigned_clusters`。
4. 汇总未分配 cluster 清单与空闲实例组清单，返回。

**返回数据（Data内容）**

| 字段 | 类型 | 说明 |
|------|------|------|
| clusters | []object | 全部 `balance_mode=EPP` 的 cluster 的分配展开视图（含未分配 cluster） |
| clusters[].cluster | string | cluster 名 |
| clusters[].group | string | 分配的实例组名；未分配时为 `null` |
| clusters[].primary | object \| null | 主实例；未分配或 degraded 时为 `null`。结构同 /epp-pool 实例：`{id, host, port}` |
| clusters[].degraded | bool | 异常标记：primary 不在实例池（实例已被移除）或组不存在时为 `true` |
| unassigned_clusters | []string | `balance_mode=EPP` 但 `epp_assignments` 无有效记录的 cluster 名列表。这些 cluster 导出 server_data_conf 时会**降级为 `WRR`**（无 EPP 实例可调度），api 同时输出 error 日志，属需尽快处理的异常态，单独列出便于巡检 |
| idle_groups | []string | 实例池中未承担任何 cluster 分配的组名列表（组内实例均未作为 primary 出现；备角色不计入占用）。扩容新组后、完成分配前会出现在此列表 |

> **说明**：视图是 `epp_assignments`（cluster→{group, primary}）与 `epp_instances`（/epp-pool）的**读时 join**，不引入冗余存储。
>
> **说明**：同一实例在多个 cluster 间复用（如 `epp-a` 同时担任 cluster-a 与 cluster-b 的 primary）是**合法**的——分配以 cluster 为单位，不做组↔cluster 的一对一约束。

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "clusters": [
            {
                "cluster": "cluster-a",
                "group": "g1",
                "primary":   { "id": "epp-a", "host": "10.0.0.1", "port": 9002 }
            },
            {
                "cluster": "cluster-b",
                "group": "g1",
                "primary":   { "id": "epp-a", "host": "10.0.0.1", "port": 9002 }
            },
            {
                "cluster": "cluster-c",
                "group": "g2",
                "primary":   { "id": "epp-c", "host": "10.0.0.3", "port": 9002 }
            }
        ],
        "unassigned_clusters": ["cluster-d", "cluster-e"],
        "idle_groups": ["g3"]
    }
}
```

### 2.2 手工覆写单个 cluster 的分配

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 手工覆写单个 cluster 的分配（运维干预入口，触发 server_data_conf version bump） | - |
| 端点 | /epp-assignments/{cluster} | - |
| 版本 | v1 | - |
| method | PUT | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| cluster | string | 集群名 | Y | - | 必填；类型为 [ClusterName](./00-common.md#15-集群名称clustername)；必须引用已存在且 `balance_mode=EPP` 的集群 |

**输入参数（Body）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| group_name | string | 目标实例组 | Y | - | 必填；非空；必须存在于 `/epp-pool` 当前实例池中 |
| primary_instance_id | string | 主实例 id | Y | - | 必填；非空；必须存在于 `group_name` 组的实例列表中 |

**HTTP BODY参数示例**

```json
{
    "group_name": "g1",
    "primary_instance_id": "epp-a"
}
```

**约束**

- `group_name` 必须存在于 `/epp-pool` 当前实例池中。
- `primary_instance_id` 必须存在于 `group_name` 组的实例列表中。
- `EPP → WRR` 反向变更（见 [clusters.md](./clusters.md)）后分配记录休眠保留，不删除；再切回 `EPP` 时继续生效（如分配已悬空由 `/epp-pool` 变更时的自动修复处理）。

**执行逻辑**

1. 校验请求参数合法性（组存在、primary 存在组实例列表中）。
2. 写入/更新 `epp_assignments`（cluster 唯一键 upsert，分配只存主）。
3. bump server_data_conf version（EPPAddr 下次导出即带出；epp_data topic 在下一次导出时由 MD5 签名机制自然 bump）。
4. 返回更新后的该 cluster 分配视图（结构同 [2.1](#21-获取分配全量视图) `clusters[]` 元素）。

**返回数据（Data内容）**

结构同 [2.1](#21-获取分配全量视图) `clusters[]` 元素：`{cluster, group, primary}`。

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "cluster": "cluster-a",
        "group": "g1",
        "primary":   { "id": "epp-a", "host": "10.0.0.1", "port": 9002 }
    }
}
```

---
