# 变更摘要：EPP 默认超时参数调整（queue_ttl / no_endpoint_queue_ttl = 9s，timeout.call = 11s）

> **日期**: 2026-11-11 ｜ **状态**: 代码 ✅ · api-define ✅ · sys-design ✅ · 单测 ✅ ｜ **接口字段**: 无新增/删除，仅默认值变化

## 1. 背景

EPP 模式下存在参数约束：**`epp_runtime.timeout.call` 必须大于 `max(flow_control.queue_ttl, flow_control.no_endpoint_queue_ttl)`**。排队预算（TTL）表示请求在队列中的最长等待，调用超时（首条消息收发）若更短，排队预算尚未耗尽就会先超时，排队/驱逐语义失效（TTL 显式 `0`＝禁用驱逐时不受约束）。

调整前默认值互不自洽：

| 字段 | 调整前 | 来源 |
| - | - | - |
| `flow_control.queue_ttl` | 60s | 缺省由 EPP 决定（llm-d 默认 60 秒） |
| `flow_control.no_endpoint_queue_ttl` | 60s（跟随） | 同上 |
| `epp_runtime.timeout.call` | 3s | `DefaultEppCallTimeout` |

即：按默认值创建的 EPP 集群天然处于 `call(3s) < 60s` 的非法组合，用户必须手工调参才能得到可用配置。

## 2. 变更

| 文件 | 变更 |
| - | - |
| `model/icluster_conf/epp_runtime.go` | `DefaultEppCallTimeout`：`3s` → **`11s`**（写库时补默认并回读，存量值不变） |
| `model/epp_pool/compiler.go` | 新增 `defaultFlowControlQueueTTLSeconds = 9`；`flow_control` 存在但未配 `queue_ttl` 时显式补 `defaultRequestTTL = 9s`，`no_endpoint_queue_ttl` 未配则跟随（同为 `9s`），不再回落 llm-d 的 60s。`flow_control` 整体缺省时保持原行为（feature gate 关闭，不补 TTL） |
| `design-docs/api-define/OpenAPI接口定义/clusters.md` | `queue_ttl` / `no_endpoint_queue_ttl` 缺省改 9 秒；`timeout.call` 默认改 11s 并补充「须大于 max(TTL)」约束说明；示例同步 |
| `design-docs/api-define/InnerAPI接口定义/server-data-conf.md` | `EPPTimeout.Call` 默认 11s |
| `design-docs/sys-design/details/EPP调度对接.md` | 默认值表补 `epp_runtime.timeout.call = 11s`；`flow_control` 缺省 9s |
| `model/icluster_conf/epp_runtime_test.go`、`model/icluster_conf/epp_test.go` | 默认补齐断言由 `3s` 改 `11s` |
| `model/epp_pool/compiler_test.go` | 新增 `TestCompileEppConfig_FlowControlDefaultTTL`：flow_control 无 TTL → 9s/9s；flow_control 缺省 → 不补 |

## 3. 兼容性

- 仅调整**缺省值**：显式传参行为不变，存量集群已存值不变；影响范围为新建集群与未传参的写入。
- `queue_ttl = 0`（显式禁用驱逐）语义不变。
- WRR 模式不导出 `epp_config`，不受影响。

## 4. 验证

- `go build ./model/...` 通过。
- `go test ./model/epp_pool/... ./model/icluster_conf/...` 通过（含新增用例）。
- 待联调：新建 EPP 集群零调参保存后，导出配置应为 `defaultRequestTTL=9s`、`noEndpointRequestTTL=9s`、`EPPTimeout.Call=11s`。

## 5. 后续

- web 侧表单默认值（`flow_control` 60/60、`defaultEppRuntime().timeout.call` 3s）需同步为 9/9/11，否则 UI 创建的集群仍显式提交 60s。
- `ai-gateway-web-test` 中集群 EPP 默认值断言需同步。