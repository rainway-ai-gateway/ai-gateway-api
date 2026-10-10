# /security/reencrypt-sweeps

密钥静态加密（DB 落盘加密）的**收敛任务**接口：对库内敏感列（`providers.api_keys`、
`api_keys.api_key`）做批量重加密/解密。任务异步执行，POST 创建任务（202），GET
列表翻查历史任务，GET 按 task_id 查询状态与进度。执行语义见
`design-docs/modifications/2026-10-05-db-encryption-at-rest/`。

## 1. 数据模型

任务（只读，经 POST 创建、GET 查询；无更新/删除接口）。异步性说明：创建后立即返回（HTTP 200，body `status=running`），进度经 GET 轮询：

| 字段 | 类型 | 说明 | 可能取值 | 合法性条件 |
|------|------|------|----------|------------|
| `task_id` | string | 任务标识 | `rsp-…` | 系统生成；GET 路径入参 |
| `status` | string | 任务状态 | `running` / `succeeded` / `failed` | 系统维护；`failed` 含 `executor lost`（执行实例失联被接管） |
| `mode` | string | 行级变换方向 | `reencrypt` / `decrypt` | 创建时传入；`reencrypt`（默认）= 非目标形态行（明文/旧 keyID 密文）重写为 active 钥密文；`decrypt` = 全部密文行解密为明文，**仅用于回滚预案** |
| `dry_run` | bool | 是否只扫描统计 | true / false | 创建时传入，默认 false；`true` 不写库 |
| `scope` | string | 扫描范围 | `all` / `providers` / `api_keys` | 创建时传入，默认 `all` |
| `active_key_id` | int | 任务启动时 keyring 的 active 钥 | 0–255 | 系统记录；`decrypt` 模式为 0。**触发响应携带以便立即核对"重写目标钥"** |
| `created_by` | string | 触发者名称 | - | 系统记录，未识别触发者时为空串；列表排障追溯用 |
| `started_at` / `finished_at` | string | 起止时间（RFC3339） | - | 系统维护；running 时 `finished_at` 为空 |
| `duration_ms` | int | 任务耗时（毫秒） | ≥0 | 完成时写入最终耗时；running 时尚未写入（为 0） |
| `summary` | object | 按表分组计数，结构见下 | - | 系统维护；**完成时按表分组落库**，running 时为空对象 |
| `error` | string | 失败归因 | - | `failed` 时为可归因错误；dry-run 任务（`status=succeeded`）固定为 `dry_run`；其余为空；**不含任何密钥或密文材料** |

`summary` 结构：

```json
{
  "providers": { "scanned": 120,  "rewritten": 118,  "skipped": 2 },
  "api_keys":  { "scanned": 2400, "rewritten": 2390, "skipped": 10 }
}
```

| 字段 | 说明 |
|------|------|
| `scanned` | 已扫描行数 |
| `rewritten` | 已重写行数；`dry_run` 下为"将要重写"数 |
| `skipped` | 已是目标形态（reencrypt：active-keyID 密文；decrypt：明文） |

**约束**

- **集群级单例互斥**：任一时刻仅一个任务运行；运行中重复触发返回 409 并携带
  持有者 `task_id`（直接轮询该任务，不排队）。
- **幂等**：任务完成后可重触发（空跑 `rewritten=0` 即收敛确认）；执行实例失联
  （心跳超时）由后续触发自动接管，旧任务标记 `executor lost`。
- 任务记录持久化于 `keyrotate_sweep_tasks` 表，历史保留（无 TTL）；多实例部署下
  任意实例均可查询任意任务。
- 权限：Feature `FeatureSecurity`（scope=System），POST 需 `update`、GET 需
  `read`；触发与完成均写操作日志（含 `mode`/`dry_run`/`scope` 与结果计数）。

## 2. POST /open-api/v1/security/reencrypt-sweeps

触发收敛任务。

**请求体**（JSON，全可选）：

| 字段 | 类型 | 必填 | 默认 | 合法性条件 |
|------|------|------|------|------------|
| `mode` | string | N | `reencrypt` | 仅 `reencrypt` \| `decrypt` |
| `dry_run` | bool | N | `false` | - |
| `scope` | string | N | `all` | 仅 `all` \| `providers` \| `api_keys` |

**成功响应**：HTTP 200，`ErrNum=200`（`xreq` 将 2xx 统一归一为 200，异步语义由 body `status=running` 表达），`Data` 为任务对象（此时 `status=running`，
含 `task_id`、`mode`、`dry_run`、`scope`、`active_key_id`）。

**错误码**：401（未认证）；402（无 FeatureSecurity 权限）；409（已有任务运行，
归因含持有者 `task_id`）；422（`mode`/`scope` 非法，字段级归因）。

## 3. GET /open-api/v1/security/reencrypt-sweeps

查询收敛任务历史列表（任务记录持久化、历史保留无 TTL，见 §1 约束）。

**Query 参数**（全可选）：

| 参数 | 类型 | 默认 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| `status` | string | - | 按任务状态过滤 | 仅 `running` \| `succeeded` \| `failed` |
| `mode` | string | - | 按行级变换方向过滤 | 仅 `reencrypt` \| `decrypt` |
| `scope` | string | - | 按扫描范围过滤 | 仅 `all` \| `providers` \| `api_keys` |
| `dry_run` | bool | - | 按是否只扫描过滤 | true / false |
| `start_time` | string | - | 按 `started_at` 过滤下界（含） | 合法 RFC3339 时间 |
| `end_time` | string | - | 按 `started_at` 过滤上界（含）；早于 `start_time` 时 422 | 合法 RFC3339 时间 |
| `page` / `page_size` | int | 1 / 20 | 分页 | 参见 [00-common.md](./00-common.md) |
| `sort_by` / `sort_order` | string | `started_at` / `desc` | 排序；`sort_by` 有效字段仅 `started_at`、`task_id`，其他值忽略并回落默认 | 同 [00-common.md](./00-common.md) |

**约束**

- 默认按 `started_at` 倒序（新→旧）。
- 运行中任务同样出现在列表中；其 `summary`/`duration_ms`/`finished_at`/`error`
  为库内已落库值（见 §1：running 时 `summary` 为空对象、`duration_ms` 为 0）。
  运行中任务的进度经本节列表或按 task_id 查询轮询 `status`，完成后读 `summary`。
- 只读接口，不写操作日志。

**成功响应**：HTTP 200，`Data` 结构：

| 字段 | 类型 | 说明 |
|------|------|------|
| `list` | array | 任务对象列表，元素字段见 §1（含 `created_by`） |
| `pagination.page` | int | 当前页码 |
| `pagination.page_size` | int | 当前每页条数 |
| `pagination.total` | int64 | 过滤后总记录数 |

**错误码**：401 / 402 同 POST；422（过滤值非法或 `end_time` 早于 `start_time`，
字段级归因）。

## 4. GET /open-api/v1/security/reencrypt-sweeps/{task_id}

查询单个任务状态与进度。

**路径参数**：

| 参数 | 类型 | 合法性条件 |
|------|------|------------|
| `task_id` | string | 非空；POST 响应获得 |

**成功响应**：HTTP 200，`Data` 为任务对象（字段见 §1）。running 时 `summary`
为空对象、`duration_ms` 为 0（完成时才落库，见 §1）。

**错误码**：401 / 402 同 POST；404（`task_id` 不存在）。

## 5. 示例

```json
// POST → 200（异步，body status=running）
{"ErrNum": 200, "ErrMsg": "success",
 "Data": {"task_id": "rsp-01J8XQ…", "status": "running", "mode": "reencrypt",
          "dry_run": false, "scope": "all", "active_key_id": 2}}

// GET 列表 → 200（默认按 started_at 倒序；running 任务 summary 为空对象）
{"ErrNum": 200, "ErrMsg": "success",
 "Data": {"list": [
            {"task_id": "rsp-01J9AB…", "status": "running", "mode": "reencrypt",
             "dry_run": false, "scope": "all", "active_key_id": 2, "created_by": "admin",
             "started_at": "2026-10-10T09:30:00Z", "finished_at": "",
             "duration_ms": 0, "summary": {}, "error": ""},
            {"task_id": "rsp-01J8XQ…", "status": "succeeded", "mode": "reencrypt",
             "dry_run": false, "scope": "all", "active_key_id": 2, "created_by": "admin",
             "started_at": "2026-10-05T10:00:00Z", "finished_at": "2026-10-05T10:00:08Z",
             "duration_ms": 8300,
             "summary": {"providers": {"scanned": 120, "rewritten": 118, "skipped": 2},
                         "api_keys":  {"scanned": 2400, "rewritten": 2390, "skipped": 10}},
             "error": ""}],
          "pagination": {"page": 1, "page_size": 20, "total": 2}}}

// GET（succeeded）→ 200
{"ErrNum": 200, "ErrMsg": "success",
 "Data": {"task_id": "rsp-01J8XQ…", "status": "succeeded", "mode": "reencrypt",
          "dry_run": false, "scope": "all", "active_key_id": 2, "created_by": "admin",
          "started_at": "2026-10-05T10:00:00Z", "finished_at": "2026-10-05T10:00:08Z",
          "duration_ms": 8300,
          "summary": {"providers": {"scanned": 120, "rewritten": 118, "skipped": 2},
                      "api_keys":  {"scanned": 2400, "rewritten": 2390, "skipped": 10}},
          "error": ""}}
```
