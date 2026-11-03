# /ai-context-rules

AI 上下文压缩规则集合（配合 BFE `mod_ai_context` 模块：一期无损裁剪 L1–L2 + 规则改写 P2，OpenAI chat 协议）。集合级全量读写：不提供 `/{id}` 单条规则操作接口。

模块级全局调优参数（`trigger_ratio` / `keep_latest_images` / `tool_result_max_chars` / `thinking_policy` / `chars_per_token` / `image_token_estimate` / `rewrite.*`）独立于本集合维护，见 [ai-context-settings.md](./ai-context-settings.md)。

## 1. 数据模型

```json
{
  "rules": [
    {
      "cond": "req_path_in(\"/v1/chat/completions\", false)",
      "mode": "balanced",
      "max_context_tokens": 64000,
      "reserve_tokens": 8192
    },
    {
      "cond": "default_t()",
      "mode": "off"
    }
  ]
}
```

**字段说明**

| 字段 | 类型 | 说明 | 可能取值 | 合法性条件 |
|------|------|------|----------|------------|
| `rules` | array | 规则列表，**按数组顺序匹配（first-match-wins），顺序即优先级** | - | 必填；`null` 按 `[]` 处理（清空全部规则）；元素类型见下表 |
| `rules[].cond` | string | BFE 条件表达式，命中即对该请求按档位执行压缩处理 | 如 `req_path_in(...)`、`default_t()` | 必填、非空、**控制面编译校验**（照 ai-cache 同款，非法表达式 422）；BFE 侧加载期编译失败整文件拒载为最后防线 |
| `rules[].mode` | string | 压缩档位：`off` 不处理；`conservative` 仅无损裁剪层（工具结果截断/旧图裁剪/thinking 删除）；`balanced` 裁剪层 + lite 规则改写；`aggressive` 裁剪层 + full 规则改写 | `off` / `conservative` / `balanced` / `aggressive` | **必填**；缺失或非法 422（BFE 对缺失/非法 mode 整文件拒载，控制面前置拦截） |
| `rules[].max_context_tokens` | int | 预算上限覆盖（token 数）：把"可用预算的输入部分"显式钉死，替代模型表窗口参与算式 | ≥ 0 | 非必填；未传/`0` = 用模型表窗口（一期 BFE 按模型名启发式 + 128k 兜底） |
| `rules[].reserve_tokens` | int | 预留输出 token 数：压缩目标 = `min(窗口, max_context_tokens) − reserve`，给应答留生长空间 | ≥ 0 | 非必填；未传/`0` = 自动 `clamp(窗口×15%, 256, 16000)` |

**约束**

- `rules` 按数组顺序匹配（first-match-wins），数组顺序 = 导出到 BFE 的顺序；不提供 `priority` 字段。
- 无 `enabled` 字段、无 `name` 字段：提交的列表即生效集合，`cond` 即规则身份，"禁用一条规则" = 从列表移除（或显式置 `mode=off` 的兜底规则）。
- 规则 `id` 为内部排序字段，不出现在 API 请求与响应中。
- 二期字段（规则级 `override`、Defaults 内 `summary`）一期不开放：API 与表只含一期字段，phase-2 以 additive 变更引入；BFE 对未知字段忽略并计数告警（`CTX_CFG_UNKNOWN_FIELD`），升级平滑。

---

## 2. 接口清单

### 2.1 全量更新AI上下文压缩规则

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 全量更新AI上下文压缩规则集合（整体替换） | - |
| 端点 | /ai-context-rules | - |
| 版本 | v1 | - |
| method | PUT | - |
| 权限 | FeatureAIContext + ActionUpdate | - |

**输入参数（Body）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| rules | array | 规则列表 | Y | 同第1节数据模型中rules结构；数组顺序即优先级 | 必填；`null` 按 `[]` 处理；每条元素校验见第1节字段说明 |

**HTTP BODY参数示例**

```json
{
    "rules": [
        {
            "cond": "req_path_in(\"/v1/chat/completions\", false)",
            "mode": "balanced",
            "max_context_tokens": 64000,
            "reserve_tokens": 8192
        },
        {
            "cond": "default_t()",
            "mode": "off"
        }
    ]
}
```

**执行逻辑**

1. 校验参数合法性：逐条校验字段（cond 非空、mode 必填且四枚举、max_context_tokens/reserve_tokens ≥ 0）；任一元素失败整体 422，集合不变
2. 单事务内整体替换规则集合（先删除全部旧规则，再按数组顺序写入；新 `id` 自增序即优先级序）；事务失败整体回滚；整体替换对同一集合串行执行（跨 api 副本），遇并发替换或瞬态锁冲突时在有限重试后返回 `409 Conflict`（issue #295）
3. 记录操作日志（`resource_type=ai_context_rule`，`before`/`after` 为整个规则集合快照）
4. 返回结果

**返回数据（Data内容）**

字段同第1节数据模型。**不返回** `created_at`/`updated_at`：本资源为整组替换语义（每次 PUT 全删重建），逐规则时间戳无信息价值（与 `ai-cache-rules` 一致）；审计时间见 `GET /operation-logs`。

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "rules": [
            {
                "cond": "req_path_in(\"/v1/chat/completions\", false)",
                "mode": "balanced",
                "max_context_tokens": 64000,
                "reserve_tokens": 8192
            },
            {
                "cond": "default_t()",
                "mode": "off",
                "max_context_tokens": 0,
                "reserve_tokens": 0
            }
        ]
    }
}
```

**约束**

- 不在提交列表中的规则即删除；`{"rules": []}` 表示清空全部规则（BFE 侧无规则命中，全部请求天然放行）。
- 本接口为整组规则的唯一写入口，配置生效时延 = conf-agent 轮询周期（`ReloadIntervalMs`）+ BFE 热加载时间。
- 校验失败（4xx）同样记录一条失败操作日志（`resource_type=ai_context_rule`、身份固定为集合、before 快照取自库中现状而非请求体）。
- 并发整体替换：同一集合的 PUT 串行执行（MySQL 命名锁；SQLite 由单写者天然串行）。若 10s 内未取得执行权，或瞬态锁冲突（MySQL 1213/1205）重试 3 次仍失败，返回 `409 Conflict`（可重试，集合保持原状，`ErrMsg` 不含数据库错误原文）：

  ```json
  {"ErrNum": 409, "ErrMsg": "Conflict: collection is being replaced concurrently, retry later"}
  ```

---

### 2.2 查询AI上下文压缩规则

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 全量查询AI上下文压缩规则集合 | - |
| 端点 | /ai-context-rules | - |
| 版本 | v1 | - |
| method | GET | - |
| 权限 | FeatureAIContext + ActionRead | - |

**返回数据（Data内容）**

字段同第1节数据模型（**不含** `created_at`/`updated_at`，见 PUT 节约束）。空集合返回 `{"rules": []}`；按优先级升序返回（顺序同导出到 BFE 的顺序）。
