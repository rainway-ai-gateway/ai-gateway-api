# /traffic-mirror-rules

流量镜像规则集合（配合 BFE `mod_traffic_mirror` 模块）。生产请求正常转发的同时，把请求副本按规则异步发往镜像目标集群（shadow traffic），响应读空丢弃、仅入统计。集合级全量读写：不提供 `/{id}` 单条规则操作接口。

## 1. 数据模型

```json
{
  "rules": [
    {
      "name": "mirror-gpt4o-to-shadow-v2",
      "cond": "req_path_prefix_in(\"/v1/chat/completions\", true) && req_body_json_in(\"model\", \"gpt-4o\", false)",
      "mirror_cluster": "cluster_shadow_v2",
      "percentage": 10,
      "set_headers": {"X-Shadow-Env": "pre-release"},
      "body_rewrites": [
        {"path": "model", "value": "deepseek-v3"}
      ]
    },
    {
      "name": "mirror-all-chat-fallback-drill",
      "cond": "req_path_prefix_in(\"/v1/chat/completions\", true)",
      "mirror_cluster": "cluster_fallback_drill",
      "percentage": 1,
      "remove_headers": ["Authorization", "Cookie", "X-Api-Key", "X-Custom-Secret"]
    },
    {
      "name": "mirror-all-to-standby",
      "cond": "default_t()",
      "mirror_cluster": "cluster_standby",
      "percentage": 100,
      "path_rewrite": "/v1/internal/chat/completions"
    }
  ]
}
```

三条规则分别演示：`set_headers`/`body_rewrites`（新模型双跑）、显式扩展 `remove_headers` 黑名单（兜底演练）、`cond` 显式 `default_t()`（全匹配）+ `path_rewrite` + `remove_headers` 缺省（由服务端填默认黑名单）。

**字段说明**

| 字段 | 类型 | 说明 | 可能取值 | 合法性条件 |
|------|------|------|----------|------------|
| `rules` | array | 规则列表，**按数组顺序匹配（first-match-wins），顺序即优先级** | - | 必填；`null` 按 `[]` 处理（清空全部规则，即镜像总开关关闭）；元素类型见下表 |
| `rules[].name` | string | 规则名称（可读性/审计用，非寻址键） | 自定义 | 必填；1-128 字符；同一集合内不得重复 |
| `rules[].cond` | string | BFE 条件表达式，命中即对该请求启用镜像；**全部匹配须显式写 `default_t()`**（与 `/ai-cache-rules` 一致，空串/缺省不接受） | 如 `default_t()`、`req_path_prefix_in(...)`、`req_body_json_in("model", ...)` 组合 | **必填**；必须能通过 BFE `condition.Build` 编译；同一集合内不得重复 |
| `rules[].mirror_cluster` | string | 镜像目标 cluster 名（引用现有 cluster） | 控制面已创建的 cluster 名 | 必填；1-128 字符；服务端校验存在性（不存在 → 422） |
| `rules[].percentage` | int | 镜像采样百分比；`0` = 命中但不采样（临时停用单条规则的推荐方式） | 0-100 | 非必填；未传时默认 `100`（全量镜像；建议前端表单引导保守值） |
| `rules[].remove_headers` | array | 镜像副本剔除的敏感 Header 黑名单。**缺省（未提交该字段）= 服务端填默认黑名单 `["Authorization","Cookie","X-Api-Key"]`；显式提交空数组 `[]` = 不剔除** | Header 名数组 | 非必填；元素非空 |
| `rules[].set_headers` | object | 镜像副本注入的自定义 Header（key/value） | 自定义 | 非必填；key/value 非空；`X-Bfe-Mirror` 由 BFE 在缺失时兜底注入 `true`，不建议在此配置同名 key |
| `rules[].body_rewrites` | array | body JSON 字段改写（新模型双跑验证），元素为 `{"path","value"}` | `[{"path":"model","value":"deepseek-v3"}]` | 非必填；元素 `path` 必填且**一期仅允许 `"model"`**；`value` 必填非空 |
| `rules[].path_rewrite` | string | 镜像路径整体替换（query 保留）；空串/缺省 = 不改写 | 如 `/v1/internal/chat/completions` | 非必填；非空时必须以 `/` 开头 |

**响应只读字段**（仅 GET/PUT 响应携带，提交时忽略）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `rules[].created_at` | string | 创建时间（RFC3339） |
| `rules[].updated_at` | string | 更新时间（RFC3339） |

**约束**

- `rules` 按数组顺序匹配（first-match-wins），数组顺序 = 导出到 BFE 的顺序；不提供 `priority` 字段。
- 无 `enabled` 字段：提交的列表即生效集合，"禁用一条规则" = 从列表移除；单条临时停用 = `percentage=0`。
- `cond` **必填**，全部匹配显式写 `default_t()`（与 `/ai-cache-rules` 一致）：避免空串被误读为"漏传"而静默全量镜像（双倍推理成本）；两条规则 `cond` 相同（如都写 `default_t()`）属集合内重复，**422 拒绝**（BFE 同 product 内拒绝重复 cond）。
- 规则 `id` 为内部排序字段，不出现在 API 请求与响应中。
- 镜像目标集群的鉴权凭证**不在规则内配置**（一期目标集群走免鉴权白名单 + 网络隔离）；模块级调参（分层超时/并发上限/熔断阈值）不在本 API 暴露，由 BFE 静态 `mod_traffic_mirror.conf` 下发。
- 镜像请求是网关内部子请求：不触发配额扣减、不重复鉴权、不计入客户用量；访问日志记录 `mirror_hit`/`mirror_cluster` 标识。

---

## 2. 接口清单

### 2.1 全量更新流量镜像规则

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 全量更新流量镜像规则集合（整体替换） | - |
| 端点 | /traffic-mirror-rules | - |
| 版本 | v1 | - |
| method | PUT | - |
| 权限 | FeatureTrafficMirror + ActionUpdate | - |

**输入参数（Body）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| rules | array | 规则列表 | Y | 同第1节数据模型中rules结构；数组顺序即优先级 | 必填；`null` 按 `[]` 处理；每条元素校验见第1节字段说明 |

**HTTP BODY参数示例**

```json
{
    "rules": [
        {
            "name": "mirror-gpt4o-to-shadow-v2",
            "cond": "req_path_prefix_in(\"/v1/chat/completions\", true) && req_body_json_in(\"model\", \"gpt-4o\", false)",
            "mirror_cluster": "cluster_shadow_v2",
            "percentage": 10,
            "set_headers": {"X-Shadow-Env": "pre-release"},
            "body_rewrites": [
                {"path": "model", "value": "deepseek-v3"}
            ]
        },
        {
            "name": "mirror-all-chat-fallback-drill",
            "cond": "req_path_prefix_in(\"/v1/chat/completions\", true)",
            "mirror_cluster": "cluster_fallback_drill",
            "percentage": 1,
            "remove_headers": ["Authorization", "Cookie", "X-Api-Key", "X-Custom-Secret"]
        },
        {
            "name": "mirror-all-to-standby",
            "cond": "default_t()",
            "mirror_cluster": "cluster_standby",
            "percentage": 100,
            "path_rewrite": "/v1/internal/chat/completions"
        }
    ]
}
```

**执行逻辑**

1. 校验参数合法性：逐条校验字段（name 格式、cond 必填且编译通过、percentage 范围、remove_headers/set_headers/body_rewrites/path_rewrite 取值）
2. 校验 `rules` 中规则名称是否重复、cond 是否重复
3. 校验每条规则的 `mirror_cluster` 是否已存在（引用校验，不存在 → 422）
4. 单事务内整体替换规则集合（先删除全部旧规则，再按数组顺序写入；新 `id` 自增序即优先级序）；任一校验失败或事务失败则整体回滚，集合保持原状；整体替换对同一集合串行执行（跨 api 副本），遇并发替换或瞬态锁冲突时在有限重试后返回 `409 Conflict`（issue #295）
5. 记录操作日志（`resource_type=traffic_mirror_rule`，`before`/`after` 为整个规则集合快照）
6. 返回结果

**返回数据（Data内容）**

字段同第1节数据模型（含响应只读字段 `created_at`/`updated_at`）。

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "rules": [
            {
                "name": "mirror-gpt4o-to-shadow-v2",
                "cond": "req_path_prefix_in(\"/v1/chat/completions\", true) && req_body_json_in(\"model\", \"gpt-4o\", false)",
                "mirror_cluster": "cluster_shadow_v2",
                "percentage": 10,
                "remove_headers": ["Authorization", "Cookie", "X-Api-Key"],
                "set_headers": {"X-Shadow-Env": "pre-release"},
                "body_rewrites": [
                    {"path": "model", "value": "deepseek-v3"}
                ],
                "path_rewrite": "",
                "created_at": "2026-09-25T10:30:00+08:00",
                "updated_at": "2026-09-25T10:30:00+08:00"
            },
            {
                "name": "mirror-all-chat-fallback-drill",
                "cond": "req_path_prefix_in(\"/v1/chat/completions\", true)",
                "mirror_cluster": "cluster_fallback_drill",
                "percentage": 1,
                "remove_headers": ["Authorization", "Cookie", "X-Api-Key", "X-Custom-Secret"],
                "set_headers": {},
                "body_rewrites": [],
                "path_rewrite": "",
                "created_at": "2026-09-25T10:30:00+08:00",
                "updated_at": "2026-09-25T10:30:00+08:00"
            },
            {
                "name": "mirror-all-to-standby",
                "cond": "default_t()",
                "mirror_cluster": "cluster_standby",
                "percentage": 100,
                "remove_headers": ["Authorization", "Cookie", "X-Api-Key"],
                "set_headers": {},
                "body_rewrites": [],
                "path_rewrite": "/v1/internal/chat/completions",
                "created_at": "2026-09-25T10:30:00+08:00",
                "updated_at": "2026-09-25T10:30:00+08:00"
            }
        ]
    }
}
```

> 响应中规则 1 与规则 3 的 `remove_headers` 均为服务端缺省填充的默认黑名单（请求中未提交该字段）；规则 2 为调用方显式提交的扩展黑名单。三条规则的 `cond` 各不相同（全匹配以显式 `default_t()` 表达）。

**约束**

- 不在提交列表中的规则即删除；`{"rules": []}` 表示清空全部规则（BFE 侧无规则命中，零镜像流量）。
- 本接口为整组规则的唯一写入口，配置生效时延 = conf-agent 轮询周期（`ReloadIntervalMs`）+ BFE 热加载时间。
- 校验失败（4xx）同样记录一条失败操作日志（`resource_type=traffic_mirror_rule`、身份固定为集合、before 快照取自库中现状而非请求体）。
- 并发整体替换：同一集合的 PUT 串行执行（MySQL 命名锁；SQLite 由单写者天然串行）。若 10s 内未取得执行权，或瞬态锁冲突（MySQL 1213/1205）重试 3 次仍失败，返回 `409 Conflict`（可重试，集合保持原状，`ErrMsg` 不含数据库错误原文）：

  ```json
  {"ErrNum": 409, "ErrMsg": "Conflict: collection is being replaced concurrently, retry later"}
  ```

---

### 2.2 查询流量镜像规则

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 全量查询流量镜像规则集合 | - |
| 端点 | /traffic-mirror-rules | - |
| 版本 | v1 | - |
| method | GET | - |
| 权限 | FeatureTrafficMirror + ActionRead | - |

**返回数据（Data内容）**

字段同第1节数据模型（含响应只读字段）。空集合返回 `{"rules": []}`；按优先级升序返回（顺序同导出到 BFE 的顺序）。
