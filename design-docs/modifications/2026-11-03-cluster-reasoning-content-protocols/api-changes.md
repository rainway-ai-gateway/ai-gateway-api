# 思考模式推理内容补齐（ensure_reasoning_content）——接口变更

## 1. 变更范围

影响以下 OpenAPI 与 InnerAPI 接口：

- `POST /clusters`
- `PATCH /clusters/{name}`
- `GET /clusters`
- `GET /clusters/{name}`
- `GET /configs/tls_conf/route_rule`（InnerAPI，导出响应中 `cluster_conf` 的 `AIConf` 新增字段）

向后兼容：以上接口对**不带** `ensure_reasoning_content` 的请求与存量数据行为完全不变（字段缺省 = BFE 关闭该行为，请求逐字节透传）。

## 2. OpenAPI `/clusters` 变更

### 2.1 请求/响应数据模型扩展

在 `llm_config` 中新增 `ensure_reasoning_content` 字段：

```json
{
    "llm_config": {
        "models": ["deepseek-chat"],
        "keys": [
            {"name": "key-primary", "weight": 100}
        ],
        "provider": "deepseek",
        "ensure_reasoning_content": ["openai"]
    }
}
```

### 2.2 新增字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `ensure_reasoning_content` | `[]string` | 否 | 需要对 assistant 消息补齐 `reasoning_content` 的协议列表。取值仅 `openai`、`anthropic`；空数组或缺省表示关闭。`openai` 协议下在 `messages[]` 的 assistant 上补 `"reasoning_content": ""`；`anthropic` 协议下在 assistant `content` 数组头部插入空 thinking 块 |

约束：

- 每个元素必须属于该 cluster 所用 provider 的 `model_protocols`（provider 的 `model_protocols` 为空时按 `["openai"]` 对待）；
- 不允许空字符串与重复项；
- `gemini` 不被接受（其请求体无 `reasoning_content` 落点，见 `change-summary.md` 决策表）。

### 2.3 行为说明（更新语义）

- `llm_config` 在 `PATCH /clusters/{name}` 中是**整体替换**语义（与 `llm_config.keys` 一致）：提交时省略 `ensure_reasoning_content` 等价于清空——即关闭该行为；
- 回滚手段：`PATCH` 提交 `"ensure_reasoning_content": []`（或省略该字段），导出中随即不再出现该字段，BFE 侧行为立即恢复；
- 写入后经 `route_rule` 导出主题下发（`cluster_conf.data`），随 conf-agent 热加载生效，无需重启 BFE。

### 2.4 响应示例（`GET /clusters/{name}` 片段）

```json
{
    "name": "cluster_deepseek",
    "llm_config": {
        "provider": "deepseek",
        "models": ["deepseek-chat", "deepseek-reasoner"],
        "model_protocols": ["openai", "anthropic"],
        "ensure_reasoning_content": ["openai"]
    }
}
```

> 注意 `model_protocols` 来自 provider 记录（只读视角），`ensure_reasoning_content` 是本 cluster 的开关，两者是"支持什么协议"与"对哪些协议做补齐"的区别。

## 3. InnerAPI 导出变更

`cluster_conf.data` 中该 cluster 的片段（键名与 BFE 结构一致，PascalCase）：

```json
{
    "cluster_deepseek": {
        "AIConf": {
            "Provider": "deepseek",
            "ModelProtocols": ["openai", "anthropic"],
            "EnsureReasoningContent": ["openai"]
        }
    }
}
```

- 未配置时该键为 `null`（不合成空数组），与 `NormalizeUpstreamError` 实测一致——导出结构体不带 `omitempty`，BFE 反序列化为 nil 即关闭；
- 导出框架（`model/imods`、`model/iversion_control`）对该字段无感知，随 `cluster_conf.BfeClusterConf` 结构体透传。

## 4. 错误码

均为参数错误（HTTP 4xx，`xerror.WrapParamErrorWithMsg`）：

| 场景 | 提示 |
|------|------|
| 含未知协议名或 `gemini` | `llm_config.ensure_reasoning_content contains unsupported protocol "gemini" (only openai, anthropic)` |
| 含空字符串 / 重复项 | `llm_config.ensure_reasoning_content must not contain empty or duplicate entries` |
| 不在 provider 申报的协议内 | `ensure_reasoning_content "anthropic" not supported by provider deepseek (model_protocols=[openai])` |