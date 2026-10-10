# API 变更说明：reencrypt-sweeps 补充 GET 列表端点（security）

> 变更摘要与决策依据见同目录《change-summary.md》。
> 本文只描述 API 契约变更：**新增 1 个端点 + 任务对象新增 1 个响应字段 +
> 三处文档对齐**，无 POST / GET-by-task_id 的结构变更。
> 响应包装遵循 `api-define/OpenAPI接口定义/00-common.md`（顶层 `ErrNum`/`ErrMsg`/`Data`，
> 成功 `ErrNum=200`）。
> 实现阶段按本文落 `api-define/OpenAPI接口定义/security.md`（六步法 Step 3）。

---

## 1. 变更概览

| 变更 | Method | Path | 说明 |
|------|--------|------|------|
| 新增 | GET | `/open-api/v1/security/reencrypt-sweeps` | 收敛任务历史列表（过滤 + 分页 + 排序） |
| 修改 | GET | `/open-api/v1/security/reencrypt-sweeps/{task_id}` | 响应新增 `created_by`（其余不变） |

命名沿用复数集合：`GET /security/reencrypt-sweeps` 读集合、`GET .../{task_id}` 读集合中单个
任务，与既有约定一致（POST 创建仍在集合上）。

## 2. 公共约定

| 项 | 约定 |
|----|------|
| 鉴权 | Feature `FeatureSecurity`（scope=System）的 `read` 动作，不足返回 402（与 GET by task_id 相同） |
| 审计 | 列表查询为只读，不写操作日志；审计留存仍由触发/完成时的操作日志承担 |
| 多实例 | 列表直接查询持久化表 `keyrotate_sweep_tasks`，任意实例均可查全量历史 |

## 3. GET /open-api/v1/security/reencrypt-sweeps

查询收敛任务历史列表。任务记录持久化、历史保留无 TTL，本接口即全量历史的翻查入口。

### Query 参数（全可选）

| 参数 | 类型 | 默认 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| `status` | string | - | 按任务状态过滤 | 仅 `running` / `succeeded` / `failed` |
| `mode` | string | - | 按行级变换方向过滤 | 仅 `reencrypt` / `decrypt` |
| `scope` | string | - | 按扫描范围过滤 | 仅 `all` / `providers` / `api_keys` |
| `dry_run` | bool | - | 按是否只扫描过滤 | true / false |
| `start_time` | string | - | 按 `started_at` 过滤下界（含） | 合法 RFC3339 时间 |
| `end_time` | string | - | 按 `started_at` 过滤上界（含） | 合法 RFC3339 时间；早于 `start_time` 时 422 |
| `page` | int | 1 | 页码 | 参见 `00-common.md`（≤0 用默认 1） |
| `page_size` | int | 20 | 每页条数 | 参见 `00-common.md`（1-100，超出截断为 100） |
| `sort_by` | string | `started_at` | 排序字段 | 仅 `started_at` / `task_id`；其他值忽略并回落默认 |
| `sort_order` | string | `desc` | 排序方向 | 同 `00-common.md`（仅 `asc`/`desc` 有效） |

### 约束

- 默认按 `started_at` 倒序（新→旧），与运维"先看最近一次收敛"的翻查习惯一致。
- **运行中任务同样出现在列表中**，其字段为库内已落库值：`summary` 完成时才按表分组
  落库（运行中为空对象 `{}`）、`duration_ms` 完成时写入最终值（运行中为 0）、
  `finished_at` 为空、`error` 为空串。运行中任务的进度观测以 GET by task_id 轮询
  `status` 翻转、完成后读 `summary` 为准。
- dry-run 任务 `status=succeeded`，`error` 固定为 `"dry_run"`（非空，非失败）。
- `pagination.total` 为过滤后的总记录数。

### 成功响应（HTTP 200）

`Data` 结构：

| 字段 | 类型 | 说明 |
|------|------|------|
| `list` | array | 任务对象列表，元素字段同 `security.md` §1（含 `created_by`），含完成任务的 `summary` 计数 |
| `pagination.page` | int | 当前页码 |
| `pagination.page_size` | int | 当前每页条数 |
| `pagination.total` | int64 | 过滤后总记录数 |

```json
{
  "ErrNum": 200, "ErrMsg": "success",
  "Data": {
    "list": [
      {
        "task_id": "rsp-01J9AB…", "status": "running", "mode": "reencrypt",
        "dry_run": false, "scope": "all", "active_key_id": 2, "created_by": "admin",
        "started_at": "2026-10-10T09:30:00Z", "finished_at": "",
        "duration_ms": 0, "summary": {}, "error": ""
      },
      {
        "task_id": "rsp-01J8XQ…", "status": "succeeded", "mode": "reencrypt",
        "dry_run": false, "scope": "all", "active_key_id": 2, "created_by": "admin",
        "started_at": "2026-10-05T10:00:00Z", "finished_at": "2026-10-05T10:00:08Z",
        "duration_ms": 8300,
        "summary": {
          "providers": { "scanned": 120,  "rewritten": 118,  "skipped": 2 },
          "api_keys":  { "scanned": 2400, "rewritten": 2390, "skipped": 10 }
        },
        "error": ""
      }
    ],
    "pagination": { "page": 1, "page_size": 20, "total": 2 }
  }
}
```

### 错误码

| HTTP/ErrNum | 场景 | ErrMsg 归因 |
|-------------|------|-------------|
| 401 | 未认证 | 既有鉴权语义 |
| 402 | 无 FeatureSecurity 权限 | "Authorizate Fail: Feature Access Deny" |
| 422 | `status`/`mode`/`scope`/`dry_run`/时间格式非法，`end_time` 早于 `start_time` | 字段级归因 |

## 4. 任务对象变更：`created_by`

`security.md` §1 数据模型新增字段（列表与 GET by task_id 均返回；POST 触发响应**不变**）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `created_by` | string | 任务触发者名称；系统记录，未识别触发者时为空串；**不含任何密钥或密文材料** |

背景：该信息本已持久化（`keyrotate_sweep_tasks.created_by`，
`model/keyrotate/types.go` `SweepTask.CreatedBy`），本次仅对外暴露。

## 5. 文档对齐（security.md 顺带修正）

三处既有表述与实现对齐（决策与代码证据见 change-summary.md §5），均为**文档修正、
实现行为不变**：

1. §1 `duration_ms`：删除"running 时为当前已耗时"，改为完成时写入最终值、running 为 0；
2. §3（GET by task_id）"含实时 `summary` 与 `duration_ms`"：改为如实描述——`summary`
   完成时按表分组落库，running 时为空对象；进度经轮询观察状态翻转；
3. §1 `error`："仅 `failed` 时非空"修正为——`failed` 时为可归因错误；**dry-run 成功任务
   固定为 `"dry_run"`**（与 change-summary 决策 5 一致）。

## 6. 兼容性

- 无既有端点路径、请求结构变更；GET by task_id 仅**新增** `created_by` 字段（加法兼容）。
- 无错误码新增（401/402/422 为既有语义）；`00-common.md` 错误码表与公共类型无需改动。
- 无 DDL 变更、无数据迁移。
