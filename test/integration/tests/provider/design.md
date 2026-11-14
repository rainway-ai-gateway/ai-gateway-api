# Provider 测试用例设计文档

## 1. 模块概述

Provider 模块负责 AI 网关**模型提供商**的管理，包括创建、查询、更新、删除，以及触发模型发现。

Provider 与 Cluster 概念分离后：

- Provider 承载后端实例池 `instance_pool`、真实 API-Key `keys`、支持的模型 `models`、模型访问协议 `model_protocols`、模型发现端点 `model_endpoint`。
- Cluster 通过 `llm_config.provider` 引用已有 Provider，并通过 `llm_config.keys` 声明 `{name, weight}` 数组选择 Provider 下要使用的 Key。
- 删除 Provider 前，会检查是否有 Cluster 或 Model-Price 记录引用该 Provider；若存在引用，则删除被拒绝（返回 409）。
- 更新 Provider 时，若移除被 Cluster 引用的 Key 或 Model，也会被拒绝（返回 409）。

## 2. 接口列表

| 编号 | 接口名称 | 方法 | 路径 | 说明 |
|------|----------|------|------|------|
| PV-1 | 创建 Provider | POST | `/open-api/v1/providers` | 创建模型提供商 |
| PV-2 | 查询 Provider 列表 | GET | `/open-api/v1/providers` | 未携带 `page`/`page_size` 时返回全部；携带时按分页返回；支持 `model_protocol` 过滤 |
| PV-3 | 查询 Provider 详情 | GET | `/open-api/v1/providers/{provider_name}` | - |
| PV-4 | 更新 Provider | PATCH | `/open-api/v1/providers/{provider_name}` | 部分更新：未提供的字段保留原值；显式提供的 `keys`、`models` 等数组字段按全量替换处理 |
| PV-5 | 删除 Provider | DELETE | `/open-api/v1/providers/{provider_name}` | 删除前检查 cluster/model-price 引用 |
| PV-6 | 触发模型发现 | POST | `/open-api/v1/providers/tools/discover-models` | 无状态工具接口，不绑定 Provider |
| PV-7 | 获取所有 Provider 名称 | GET | `/open-api/v1/providers/actions/get-provider-names` | 返回全量 provider 名称列表 |
| PV-8 | 设置高峰/闲时模板 | PUT | `/open-api/v1/providers/{provider_name}/pricing-tiers` | 支持 JSON / YAML / multipart 上传 |

## 3. 测试用例统计

| 接口 / 场景 | 测试用例数 |
|-------------|-----------|
| 创建 Provider | 20 |
| 查询 Provider 列表 | 3 |
| 查询 Provider 详情 | 3 |
| 更新 Provider | 12 |
| Provider instance_pool 同步到 Inner API | 5 |
| 删除 Provider | 4 |
| 触发模型发现 | 6 |
| 获取所有 Provider 名称 | 1 |
| instance_pool 默认 name 生成 | 1 |
| 设置高峰/闲时模板 | 11 |
| K8s 实例池（/k8s_pools + k8s_pool 模式 Provider） | 12 |
| **合计** | **78** |

## 4. 认证方式

测试环境配置 `SkipTokenValidate=true`，所有请求无需携带认证头。

## 5. 目录结构

```
provider/
├── design.md
├── create/
│   └── create_test.go
├── list/
│   └── list_test.go
├── one/
│   └── one_test.go
├── update/
│   └── update_test.go
├── instance_pool_sync/
│   └── instance_pool_sync_test.go
├── k8s_pool/
│   └── k8s_pool_test.go
├── delete/
│   └── delete_test.go
├── discover/
│   └── discover_test.go
├── list_names/
│   └── list_names_test.go
├── instance_name_default/
│   └── instance_name_default_test.go
└── pricing_tiers/
    └── pricing_tiers_test.go
```

## 6. 创建 Provider

### 6.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Provider |
| 接口名称 | 创建 Provider |
| 方法 | POST |
| 路径 | `/open-api/v1/providers` |
| 说明 | 创建模型提供商，返回完整 Provider 对象 |

### 6.2 接口参数说明

#### 6.2.1 请求参数

##### Body 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `name` | string | Y | Provider 唯一标识，全局唯一 |
| `description` | string | N | 描述信息，长度 0-256 |
| `model_endpoint` | object | N | 模型发现端点，默认 `{schema:"https", uri:"/v1/models"}` |
| `models` | []string | N | 支持的模型列表，元素非空且不可重复 |
| `keys` | []object | N | API-Key 列表，元素为 `{name, key}`；同一 Provider 内 `name` 唯一 |
| `instance_pool` | []object | Y | 后端实例池，至少 1 个元素 |
| `model_protocols` | []string | Y | 支持的模型访问协议，至少 1 个元素，枚举：`openai`、`anthropic`、`gemini` |
| `time_zone` | string | N | 计算时段所使用的时区，默认 `Asia/Shanghai` |
| `tiers` | []object | N | 时段 tier 定义列表，**初期 `name` 只支持 `peak`** |

##### `instance_pool` 元素结构

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `addr` | string | Y | 实例地址 |
| `weight` | int | Y | 权重，取值范围 [0,100]；至少有一个实例 `weight > 0` |
| `port` | int | Y | 实例端口 |

##### `keys` 元素结构

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `name` | string | Y | Key 名称，同一 Provider 内唯一，长度 1-128 |
| `key` | string | Y | API-Key 明文，长度 1-512 |

##### `tiers` 元素结构

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `name` | string | Y | Tier 名称，**初期只支持 `peak`** |
| `time_ranges` | []object | Y | 时段范围列表，命中任意一个即属于该 tier |

##### `time_ranges` 元素结构

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `weekdays` | []int | N | 星期几，0=周日，...，6=周六；为空表示每天 |
| `start` | string | Y | 开始时间，格式 `HH:MM` |
| `end` | string | Y | 结束时间，格式 `HH:MM`；`end` 必须大于 `start` |

#### 6.2.2 响应参数

| 参数名 | 类型 | 说明 |
|--------|------|------|
| `id` | int64 | Provider ID |
| `name` | string | Provider 名称 |
| `description` | string | 描述 |
| `model_endpoint` | object | 模型发现端点 |
| `models` | []string | 模型列表 |
| `keys` | []object | Key 列表（含明文） |
| `instance_pool` | []object | 实例池 |
| `model_protocols` | []string | 协议列表 |
| `time_zone` | string | 时区 |
| `tiers` | []object | 时段 tier 列表 |
| `create_time` | int64 | 创建时间 |
| `update_time` | int64 | 更新时间 |

### 6.3 测试用例

| 用例编号 | 用例名称 | 预期结果 |
|----------|----------|----------|
| PV-1-001 | 最小参数创建 Provider | 200，返回的 `description` 为空字符串，`models` 为 `["deepseek-chat"]`，`keys` 为空数组 |
| PV-1-002 | 完整参数创建 Provider | 200，返回的 `models`、`keys`、`instance_pool` 与输入一致 |
| PV-1-002a | 创建 anthropic 协议 Provider | 200，返回的 `model_protocols` 为 `["anthropic"]` |
| PV-1-002c | 创建 gemini 协议 Provider | 200，返回的 `model_protocols` 为 `["gemini"]` |
| PV-1-002b | instance_pool 不再包含 name 字段 | 200，返回的 instance 中无 `name` 字段 |
| PV-1-003 | 重复 Provider 名称 | 555 |
| PV-1-004 | 缺少 `instance_pool` | 422 |
| PV-1-005 | 缺少 `model_protocols` | 422 |
| PV-1-006 | 非法 `model_protocols` | 422 |
| PV-1-007 | `instance_pool` 为空数组 | 422 |
| PV-1-008 | 实例 `port` 非法 | 422 |
| PV-1-009 | 非法 `name` | 422 |
| PV-1-010 | `weight` 超过 100 | 422 |
| PV-1-011 | 重复实例 `(addr, port)` 组合 | 422 |
| PV-1-012 | `models` 元素重复 | 422 |
| PV-1-013 | `keys` 中 `name` 重复 | 422 |
| PV-1-014 | 创建带 `protocol_paths` 的 Provider（双协议百炼形态） | 200，返回的 `protocol_paths` 与输入一致 |
| PV-1-015 | `protocol_paths` 键未在 `model_protocols` 声明 | 422 |
| PV-1-016 | `protocol_paths` 非法协议键（如 `gemini`） | 422 |
| PV-1-017 | `protocol_paths` 值缺少 `/` 前缀 | 422 |
| PV-1-018 | `protocol_paths` 值以 `/` 结尾 | 422 |
| PV-1-019 | 缺少 `models` | 422 |
| PV-1-020 | `models` 为空数组 | 422 |

### 6.4 测试场景详细设计

#### 6.4.1 PV-1-001：最小参数创建 Provider

##### 设计思路

验证仅传入必填字段时，Provider 能够成功创建，且可选字段使用系统默认值。`models` 自 issue #115 起为必填字段（至少 1 个元素），最小参数场景同样携带。

##### 前提数据准备

1. 使用 `testutil.UniqueProviderName()` 生成全局唯一的 Provider 名称 `providerMin`。

##### 执行步骤

1. 发送 POST 请求到 `/open-api/v1/providers`，请求体仅包含 `name`、`instance_pool`、`model_protocols`、`models`。
2. 验证响应 `ErrNum = 200`。
3. 断言返回字段：
   - `name` 等于请求传入值；
   - `description` 为空字符串；
   - `models` 与请求传入值一致；
   - `keys` 为空数组；
   - `model_protocols` 为 `["openai"]`。

##### 请求参数

```json
{
  "name": "provider-xxx",
  "instance_pool": [
    {"addr": "10.0.0.1", "weight": 100, "port": 8080}
  ],
  "model_protocols": ["openai"],
  "models": ["deepseek-chat"]
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | 与请求一致 | Equals |
| description | "" | Equals |
| models | ["deepseek-chat"] | Equals |
| keys | [] | Empty |

---

#### 6.4.2 PV-1-002：完整参数创建 Provider

##### 设计思路

验证传入完整参数时，Provider 各字段（含 `model_endpoint`、`models`、`keys`）能够正确持久化并原样返回。

##### 前提数据准备

1. 生成唯一 Provider 名称 `providerFull`。

##### 执行步骤

1. 发送 POST 请求，携带 `description`、`model_endpoint`、`models`、`keys`、`instance_pool`、`model_protocols`。
2. 验证响应 `ErrNum = 200`。
3. 断言 `name`、`description`、`models` 长度、`keys` 长度、`instance_pool` 长度与输入一致。

##### 请求参数

```json
{
  "name": "provider-full-xxx",
  "description": "完整 Provider",
  "model_endpoint": {"schema": "https", "uri": "/v1/models"},
  "models": ["deepseek-chat", "deepseek-coder"],
  "keys": [
    {"name": "key-primary", "key": "sk-aaaaaaaaaaaa"},
    {"name": "key-secondary", "key": "sk-bbbbbbbbbbbb"}
  ],
  "instance_pool": [
    {"addr": "api.deepseek.com", "weight": 100, "port": 443}
  ],
  "model_protocols": ["openai"]
}
```

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | 与请求一致 | Equals |
| description | "完整 Provider" | Equals |
| models | 长度 2 | Len |
| keys | 长度 2 | Len |
| instance_pool | 长度 1 | Len |

---

#### 6.4.3 PV-1-003：重复 Provider 名称

##### 设计思路

验证 Provider 名称全局唯一，重复创建应返回业务错误 555。

##### 前提数据准备

1. 预先通过 `testutil.CreateProvider(providerDup)` 创建一个 Provider。

##### 执行步骤

1. 使用相同名称再次发送 POST 请求。
2. 验证响应 `ErrNum = 555`。

##### 预期返回结果

**ErrNum**：555

---

#### 6.4.4 PV-1-004 ~ PV-1-020：参数校验异常场景

##### 设计思路

统一覆盖创建接口的字段级校验，包括必填缺失、枚举非法、数组唯一性约束、数值范围、`protocol_paths` 键值合法性等。所有异常场景均应返回 422。

##### 典型异常参数

| 用例编号 | 异常点 | 请求体关键特征 |
|----------|--------|----------------|
| PV-1-004 | 缺少 `instance_pool` | 不包含 `instance_pool` |
| PV-1-005 | 缺少 `model_protocols` | 不包含 `model_protocols` |
| PV-1-006 | 非法 `model_protocols` | `"model_protocols": ["invalid"]` |
| PV-1-007 | `instance_pool` 为空数组 | `"instance_pool": []` |
| PV-1-008 | 实例 `port` 非法 | `port: 0` |
| PV-1-009 | 非法 `name` | `name: "-bad-name-"` |
| PV-1-010 | `weight` 超过 100 | `weight: 101` |
| PV-1-011 | 重复 `(addr, port)` | 两个实例使用相同 `addr` 与 `port` |
| PV-1-012 | `models` 元素重复 | `"models": ["m", "m"]` |
| PV-1-013 | `keys` 中 `name` 重复 | 两个 key 使用相同 `name` |
| PV-1-015 | `protocol_paths` 键未声明 | `model_protocols=["openai"]` 但配置 `anthropic` 路径 |
| PV-1-016 | `protocol_paths` 非法协议键 | 键为 `gemini`（BFE 改写公式不支持） |
| PV-1-017 | `protocol_paths` 值缺少 `/` 前缀 | `"openai": "compatible-mode/v1"` |
| PV-1-018 | `protocol_paths` 值以 `/` 结尾 | `"openai": "/compatible-mode/"` |
| PV-1-019 | 缺少 `models`（issue #115，必填） | 不包含 `models` |
| PV-1-020 | `models` 为空数组（issue #115，至少 1 个元素） | `"models": []` |

##### 预期返回结果

**ErrNum**：422

#### 6.4.5 PV-1-014：创建带 protocol_paths 的 Provider

##### 设计思路

验证 `protocol_paths`（按协议的上游路径前缀）随创建请求持久化，并在响应中原样返回。配置取百炼双协议形态：`openai=/compatible-mode/v1`、`anthropic=/apps/anthropic`。

##### 请求参数关键字段

```json
{
  "name": "provider-xxx",
  "instance_pool": [{"addr": "dashscope.aliyuncs.com", "weight": 100, "port": 443}],
  "model_protocols": ["openai", "anthropic"],
  "protocol_paths": {
    "openai": "/compatible-mode/v1",
    "anthropic": "/apps/anthropic"
  }
}
```

##### 预期返回结果

**ErrNum**：200；`protocol_paths` 为对象，`openai`/`anthropic` 键值与输入一致。


## 7. 查询 Provider 列表

### 7.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Provider |
| 接口名称 | 查询 Provider 列表 |
| 方法 | GET |
| 路径 | `/open-api/v1/providers` |
| 说明 | 支持分页与 `model_protocol` 过滤 |

### 7.2 请求参数

##### Query 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `page` | int | N | 页码；未传时与 `page_size` 同时缺失，返回全部记录 |
| `page_size` | int | N | 每页条数，最大 1000；未传时与 `page` 同时缺失，返回全部记录 |
| `model_protocol` | string | N | 按协议过滤，须为 `model_protocols` 枚举值 |

### 7.3 响应参数

始终返回 `{ list, pagination }` 结构：

- 未携带 `page`/`page_size` 时，`list` 包含全部匹配记录，`pagination.page=1`，`pagination.page_size=total`；
- 携带分页参数时，`list` 包含当前页记录，`pagination` 为对应分页信息。

```json
{
  "list": [...],
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total": 100
  }
}
```

### 7.4 测试用例

| 用例编号 | 用例名称 | 预期结果 |
|----------|----------|----------|
| PV-2-001 | 无分页参数返回全部 | 200，`pagination.total >= 2`，`list` 长度等于 `pagination.total` |
| PV-2-002 | 自定义分页 | 200，`list` 长度为 1 |
| PV-2-003 | 按 `model_protocol` 过滤 | 200，返回的 Provider 都包含 `openai` 协议 |
| PV-2-004 | 按 `model_protocol=gemini` 过滤 | 200，返回的 Provider 都包含 `gemini` 协议 |

### 7.5 测试场景详细设计

#### 7.5.1 PV-2-001：无分页参数返回全部

##### 设计思路

验证未携带分页参数时，接口返回全部 Provider 记录，且 `list` 长度与 `pagination.total` 一致。

##### 前提数据准备

1. 使用 `testutil.CreateProvider` 创建至少 2 个 Provider（一个 openai 协议，一个 anthropic 协议）。

##### 执行步骤

1. 发送 GET 请求到 `/open-api/v1/providers`，不带任何 Query 参数。
2. 验证响应 `ErrNum = 200`。
3. 断言 `pagination.total >= 2`。
4. 断言 `len(list) == pagination.total`。

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| list | 非空数组 | NotEmpty |
| pagination.total | >= 2 | GreaterOrEqual |
| pagination.page | 1 | Equals |

---

#### 7.5.2 PV-2-002：自定义分页

##### 设计思路

验证携带 `page` 与 `page_size` 时，接口按分页返回，`list` 长度等于 `page_size`（数据足够时）。

##### 执行步骤

1. 发送 GET 请求，`page=1&page_size=1`。
2. 验证响应 `ErrNum = 200`。
3. 断言 `len(list) == 1`。
4. 断言 `pagination.total >= 2`。

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| list | 长度 1 | Len |
| pagination.page_size | 1 | Equals |
| pagination.total | >= 2 | GreaterOrEqual |

---

#### 7.5.3 PV-2-003：按 `model_protocol` 过滤

##### 设计思路

验证 `model_protocol=openai` 过滤条件仅返回包含 `openai` 协议的 Provider。

##### 前提数据准备

1. 已创建 openai 协议 Provider A 与 anthropic 协议 Provider B。

##### 执行步骤

1. 发送 GET 请求，`model_protocol=openai`。
2. 验证响应 `ErrNum = 200`。
3. 遍历 `list`，断言每个 Provider 的 `model_protocols` 包含 `openai`。

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| list | 非空数组 | NotEmpty |
| list[i].model_protocols | 包含 "openai" | Contains |

#### 7.5.4 PV-2-004：按 `model_protocol=gemini` 过滤

##### 设计思路

验证 `model_protocol=gemini` 过滤条件仅返回包含 `gemini` 协议的 Provider。

##### 前提数据准备

1. 已创建 openai 协议 Provider A 与 gemini 协议 Provider C。

##### 执行步骤

1. 发送 GET 请求，`model_protocol=gemini`。
2. 验证响应 `ErrNum = 200`。
3. 遍历 `list`，断言每个 Provider 的 `model_protocols` 包含 `gemini`。

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| list | 非空数组 | NotEmpty |
| list[i].model_protocols | 包含 "gemini" | Contains |

## 8. 查询 Provider 详情

### 8.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Provider |
| 接口名称 | 查询 Provider 详情 |
| 方法 | GET |
| 路径 | `/open-api/v1/providers/{provider_name}` |

### 8.2 测试用例

| 用例编号 | 用例名称 | 预期结果 |
|----------|----------|----------|
| PV-3-001 | 查询存在的 Provider | 200，返回的 `name` 与请求一致；`time_zone` 为 `Asia/Shanghai`；`tiers` 为 `[]` |
| PV-3-002 | 查询不存在的 Provider | 404 |
| PV-3-003 | 查询已设置 pricing-tiers 的 Provider | 200，返回的 `time_zone` 和 `tiers` 与设置一致 |

### 8.3 测试场景详细设计

#### 8.3.1 PV-3-001：查询存在的 Provider

##### 设计思路

验证正常查询已创建的 Provider，默认时区与 tiers 为空。

##### 前提数据准备

1. 使用 `testutil.CreateProvider(providerName)` 创建一个 Provider。

##### 执行步骤

1. 发送 GET 请求到 `/open-api/v1/providers/{provider_name}`。
2. 验证响应 `ErrNum = 200`。
3. 断言 `name` 与请求一致，`time_zone` 为 `Asia/Shanghai`，`tiers` 为空数组。

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | 与 URI 一致 | Equals |
| time_zone | "Asia/Shanghai" | Equals |
| tiers | [] | Empty |

---

#### 8.3.2 PV-3-002：查询不存在的 Provider

##### 设计思路

验证查询不存在的 Provider 返回 404。

##### 执行步骤

1. 发送 GET 请求到 `/open-api/v1/providers/non_existent_provider`。
2. 验证响应 `ErrNum = 404`。

##### 预期返回结果

**ErrNum**：404

---

#### 8.3.3 PV-3-003：查询已设置 pricing-tiers 的 Provider

##### 设计思路

验证 pricing-tiers 设置成功后，通过 GET Provider 详情可正确返回 `time_zone` 与 `tiers`。

##### 前提数据准备

1. 创建 Provider。
2. 调用 `PUT /providers/{provider_name}/pricing-tiers` 设置 `time_zone=America/New_York` 与一个 `peak` tier。

##### 执行步骤

1. 发送 GET 请求到 Provider 详情接口。
2. 验证响应 `ErrNum = 200`。
3. 断言 `time_zone` 为 `America/New_York`，`tiers` 长度 1，首个 tier 的 `name` 为 `peak`。

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| time_zone | "America/New_York" | Equals |
| tiers | 长度 1，name=peak | Len + Equals |

## 9. 更新 Provider

### 9.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Provider |
| 接口名称 | 更新 Provider |
| 方法 | PATCH |
| 路径 | `/open-api/v1/providers/{provider_name}` |
| 说明 | 部分更新语义：请求体未提供的字段（`description`、`model_endpoint`、`models`、`keys`、`time_zone`、`tiers` 等）保留原值；显式提供的数组字段按全量替换处理 |

### 9.2 请求参数

可修改字段与创建接口一致。`provider_name` 由 URI 路径指定，请求体中无需再传 `name`（若包含 `name` 返回 422）；更新时仍需携带 `instance_pool` 和 `model_protocols`。

### 9.3 测试用例

| 用例编号 | 用例名称 | 预期结果 |
|----------|----------|----------|
| PV-4-001 | 更新 `description` | 200，`description` 更新成功 |
| PV-4-002 | 更新 `models` | 200，`models` 长度变为 3 |
| PV-4-003 | 更新 `keys`（全量替换） | 200，`keys` 被替换为新的完整列表 |
| PV-4-004 | 更新不存在的 Provider | 404 |
| PV-4-005 | 请求体不包含 `name` | 200，返回的 `name` 与 URI 一致 |
| PV-4-005a | 更新时 instance_pool 不再包含 name 字段 | 200，返回的 instance 中无 `name` 字段 |
| PV-4-006 | 请求体包含 `name` | 422 |
| PV-4-007 | 省略字段保留原值（issue #147） | 200，`description` 更新成功，未提供的 `time_zone`/`tiers`/`models`/`keys`/`model_endpoint` 全部保留原值 |
| PV-4-008 | 只更新 `time_zone` | 200，`time_zone` 更新成功，其余字段（含 `tiers`）保留原值 |
| PV-4-009 | 显式空数组清空 `keys`/`tiers` | 200，显式传入的 `keys`/`tiers` 被清空，省略的 `models`/`time_zone`/`description`/`model_endpoint` 保留原值 |
| PV-4-010 | 非法 `time_zone` | 422，且已存储的 `time_zone` 不被修改 |
| PV-4-011 | 更新 `protocol_paths`（全量替换） | 200，`protocol_paths` 被替换为新值 |
| PV-4-012 | 省略 `protocol_paths` 保持原值 | 200，未提供时保留原 `protocol_paths` |
| PV-4-013 | 显式空对象清空 `protocol_paths` | 200，`protocol_paths` 被清空 |
| PV-4-014 | 收缩 `model_protocols` 与 `protocol_paths` 同请求给出合法组合 | 200，两者同时生效 |
| PV-4-015 | 仅收缩 `model_protocols` 使存量 `protocol_paths` 非法 | 422，已存储的 `protocol_paths` 不被修改 |
| PV-4-016 | 显式空 `models` 数组（issue #115） | 422，且已存储的 `models` 不被修改 |

### 9.4 测试场景详细设计

#### 9.4.1 PV-4-001：更新 description

##### 设计思路

验证单个标量字段 `description` 可被独立更新，且不影响其他字段。

##### 前提数据准备

1. 创建 Provider。

##### 执行步骤

1. 发送 PATCH 请求，携带 `description`、必须的 `instance_pool` 与 `model_protocols`。
2. 验证响应 `ErrNum = 200`。
3. 断言返回的 `description` 为新值。

##### 请求参数

```json
{
  "description": "更新后的 Provider 描述",
  "instance_pool": [{"addr": "10.0.0.1", "weight": 100, "port": 8080}],
  "model_protocols": ["openai"]
}
```

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| description | "更新后的 Provider 描述" | Equals |

---

#### 9.4.2 PV-4-003：更新 keys（全量替换）

##### 设计思路

验证 `keys` 字段按全量替换处理，旧 Key 会被清除，新 Key 列表完全生效。

##### 前提数据准备

1. 创建 Provider，初始携带 2 个 keys。

##### 执行步骤

1. 发送 PATCH 请求，携带新的 `keys` 列表（2 个新 key，名称与旧 key 不同）。
2. 验证响应 `ErrNum = 200`。
3. 断言返回的 `keys` 长度 2，且名称为新传入值。

##### 请求参数

```json
{
  "keys": [
    {"name": "key-primary", "key": "sk-new-primary"},
    {"name": "key-tertiary", "key": "sk-cccccccccccc"}
  ],
  "instance_pool": [{"addr": "10.0.0.1", "weight": 100, "port": 8080}],
  "model_protocols": ["openai"]
}
```

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| keys | 长度 2 | Len |

---

#### 9.4.3 PV-4-004：更新不存在的 Provider

##### 设计思路

验证对不存在 Provider 的更新请求返回 404。

##### 执行步骤

1. 发送 PATCH 请求到 `/open-api/v1/providers/non_existent_provider`。
2. 验证响应 `ErrNum = 404`。

##### 预期返回结果

**ErrNum**：404

---

#### 9.4.4 PV-4-006：请求体包含 name

##### 设计思路

验证更新接口不允许在请求体中再次传入 `name`，防止误修改 Provider 标识。

##### 执行步骤

1. 发送 PATCH 请求，请求体包含 `name` 字段。
2. 验证响应 `ErrNum = 422`。

##### 预期返回结果

**ErrNum**：422

#### 9.4.5 PV-4-007 ~ PV-4-010、PV-4-016：部分更新语义（issue #147、#115）

##### 设计思路

验证 PATCH 为部分更新语义：请求体未提供的字段保留原值，显式提供的字段（含空数组）正常更新。回归 issue #147——修复前省略 `time_zone`/`tiers`/`models`/`keys`/`model_endpoint` 会被静默覆盖为默认值或清空。

自 issue #115 起 `models` 为必填字段（至少 1 个元素）：PATCH 省略 `models` 仍表示保留原值（部分更新语义不变），但显式传 `models=[]` 必须 422 拒绝（PV-4-016）——显式提供即全量替换，清空 models 违反合同且可能使已引用 cluster 失效。

测试前置：创建 Provider 时设置自定义 `time_zone=UTC`、`model_endpoint={http, /custom/models}`、`models=["deepseek-chat","deepseek-coder"]`，并通过 `PUT /providers/{name}/pricing-tiers` 设置 peak 时段模板；`keys` 使用创建时的默认两个 Key。

##### 执行步骤

1. **PV-4-007**：PATCH 请求体只含 `description`（另附校验必填的 `instance_pool`、`model_protocols`），随后 GET 详情，断言 `time_zone`/`tiers`/`models`/`keys`/`model_endpoint` 与前置值一致。
2. **PV-4-008**：PATCH 请求体只含 `time_zone=Europe/London`，断言更新成功且 `description`、`tiers`、`keys`、`model_endpoint` 保留。
3. **PV-4-009**：PATCH 请求体显式传 `keys=[]`、`tiers=[]`，断言二者被清空，而省略的 `models`/`time_zone`/`description`/`model_endpoint` 保留。
4. **PV-4-010**：PATCH 请求体传非法 `time_zone=Not/AZone`，断言返回 422，且 GET 详情确认存储值未被修改。
5. **PV-4-016**：PATCH 请求体显式传 `models=[]`，断言返回 422，且 GET 详情确认存储的 `models` 未被修改。

##### 预期返回结果

| 用例 | 预期 |
|------|------|
| PV-4-007 | 200，未提供的五项字段全部保留原值 |
| PV-4-008 | 200，`time_zone` 更新为 `Europe/London`，其余字段保留 |
| PV-4-009 | 200，显式空数组的 `keys`/`tiers` 被清空，`models` 等省略字段保留 |
| PV-4-010 | 422，存储的 `time_zone` 不被修改 |
| PV-4-016 | 422，存储的 `models` 不被修改 |

#### 9.4.4 PV-4-011 ~ PV-4-015：protocol_paths 更新语义

##### 设计思路

验证 `protocol_paths` 在 PATCH 下的部分更新语义：

- **全量替换**（PV-4-011）：显式提供时整体替换；
- **省略保留**（PV-4-012）：未提供时保持原值（DAO nil-skip）；
- **显式清空**（PV-4-013）：显式空对象 `{}` 清空（与数组字段的"显式空数组清空"语义对齐）；
- **组合调整**（PV-4-014）：收缩 `model_protocols` 时，须在同一请求中给出与其兼容的 `protocol_paths`；
- **孤儿校验**（PV-4-015）：仅收缩 `model_protocols`、省略 `protocol_paths` 时，存量 `protocol_paths` 中的被移除协议成为孤儿，必须 422 拒绝且存储值不被修改（`UpdateProvider` 对"有效 protocol_paths = 存量值"做交叉校验）。

##### 预期返回结果

| 用例 | 预期 |
|------|------|
| PV-4-011 | 200，`protocol_paths` 等于请求值 |
| PV-4-012 | 200，`protocol_paths` 保持创建时的值 |
| PV-4-013 | 200，`protocol_paths` 为空对象 |
| PV-4-014 | 200，`model_protocols` 与 `protocol_paths` 同时生效 |
| PV-4-015 | 422，存储值不被修改 |


## 10. Provider instance_pool 同步到 Inner API

### 10.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Provider / Inner API |
| 接口名称 | 修改 Provider instance_pool 后验证 cluster_table 导出 |
| 方法 | PATCH + GET |
| 路径 | `/open-api/v1/providers/{provider_name}` + `/inner-api/v1/configs/gslb_data/cluster_table` |
| 说明 | 验证 Open API 修改 provider 实例池后，Inner API 的 cluster_table 配置导出会同步更新 |

### 10.2 测试用例

| 用例编号 | 用例名称 | 预期结果 |
|----------|----------|----------|
| PV-SYNC-1-001 | 修改 provider instance_pool 后 cluster_table 同步更新 | cluster_table 中 cluster 的 backend 变为新实例 |
| PV-SYNC-1-002 | 更新 Provider 移除被 Cluster 引用的 Key | 409 |
| PV-SYNC-1-003 | 更新 Provider 移除被 Cluster 引用的 Model | 409 |
| PV-SYNC-1-004 | 更新 Provider 移除被 Cluster 引用的 Model（issue #156 事务回滚契约） | 409，且 GET Provider models 保持原值未变 |
| PV-SYNC-1-005 | 更新 Provider 移除未被引用的 Model（对照组） | 200，且 GET Provider models 仅保留 m1 |

### 10.3 测试场景详细设计

#### 10.3.1 PV-SYNC-1-001：修改 provider instance_pool 后 cluster_table 同步更新

##### 设计思路

验证 Provider 实例池修改后，Inner API 导出的 `cluster_table` 会同步反映新的后端实例，旧实例被移除。

##### 前提数据准备

1. 创建 Provider，初始 `instance_pool` 为 `[{addr: "10.0.0.1", port: 8080, weight: 100}]`。
2. 创建 Cluster，通过 `llm_config.provider` 引用该 Provider。

##### 执行步骤

1. 调用 `GET /inner-api/v1/configs/gslb_data/cluster_table`，断言 cluster 的 backends 包含 `10.0.0.1_8080`。
2. 调用 `PATCH /open-api/v1/providers/{provider_name}`，将 `instance_pool` 修改为 `[{addr: "10.0.0.2", port: 8081, weight: 100}]`。
3. 再次调用 `GET /inner-api/v1/configs/gslb_data/cluster_table`。
4. 断言 backends 包含 `10.0.0.2_8081`，且不再包含 `10.0.0.1_8080`。

##### 预期返回结果

**Open API PATCH ErrNum**：200  
**Inner API GET ErrNum**：200

**cluster_table 校验**：

| 校验项 | 预期值 | 校验方式 |
|--------|--------|---------|
| 新 backend | Name=10.0.0.2_8081, Addr=10.0.0.2, Port=8081 | Equals |
| 旧 backend | 不存在 | NotExists |

---

#### 10.3.2 PV-SYNC-1-002：更新 Provider 移除被 Cluster 引用的 Key

##### 设计思路

验证更新 Provider 时，若新 `keys` 列表不再包含被 Cluster 引用的 Key，则返回 409，避免破坏正在使用的路由配置。

##### 前提数据准备

1. 创建 Provider，携带 keys `k1`、`k2`。
2. 创建 Cluster，`llm_config.keys` 引用 `k1`。

##### 执行步骤

1. 发送 PATCH 请求，将 `keys` 更新为仅保留 `k2`。
2. 验证响应 `ErrNum = 409`。

##### 预期返回结果

**ErrNum**：409

---

#### 10.3.3 PV-SYNC-1-003：更新 Provider 移除被 Cluster 引用的 Model

##### 设计思路

验证更新 Provider 时，若新 `models` 列表不再包含被 Cluster 引用的 Model，则返回 409。

##### 前提数据准备

1. 创建 Provider，携带 models `m1`、`m2`。
2. 创建 Cluster，`llm_config.models` 引用 `m2`。

##### 执行步骤

1. 发送 PATCH 请求，将 `models` 更新为仅保留 `m1`。
2. 验证响应 `ErrNum = 409`。

##### 预期返回结果

**ErrNum**：409

#### 10.3.4 PV-SYNC-1-004：更新 Provider 移除被 Cluster 引用的 Model（issue #156 事务回滚契约）

##### 设计思路

PV-SYNC-1-003 只断言了 409 响应；本用例钉死 issue #156 的完整契约：409 时更新必须随事务回滚，Provider 记录保持原值。若 dao 语句绕过事务走连接池 autocommit（假事务），该用例的 GET 断言会失败。

##### 前提数据准备

1. 创建 Provider，携带 models `m1`、`m2`。
2. 创建 Cluster，`llm_config.models` 引用 `m2`。

##### 执行步骤

1. 发送 PATCH 请求，将 `models` 更新为仅保留 `m1`。
2. 验证响应 `ErrNum = 409`。
3. GET 查询该 Provider，验证 `models` 仍为 `m1`、`m2`（更新被回滚）。

##### 预期返回结果

**ErrNum**：409

**断言**：GET `/open-api/v1/providers/{provider_name}` 返回的 `models` 与初始值一致（ElementsMatch `m1`、`m2`）。

#### 10.3.5 PV-SYNC-1-005：更新 Provider 移除未被引用的 Model（对照组）

##### 设计思路

对照组：无引用冲突时 PATCH 应成功且变更持久化，排除"回滚逻辑误伤正常更新"的可能。

##### 前提数据准备

1. 创建 Provider，携带 models `m1`、`m2`（无任何 Cluster 引用）。

##### 执行步骤

1. 发送 PATCH 请求，将 `models` 更新为仅保留 `m1`。
2. 验证响应成功。
3. GET 查询该 Provider，验证 `models` 仅剩 `m1`。

##### 预期返回结果

**ErrNum**：200

**断言**：GET `/open-api/v1/providers/{provider_name}` 返回的 `models` 为 `m1`。

## 11. 删除 Provider

### 11.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Provider |
| 接口名称 | 删除 Provider |
| 方法 | DELETE |
| 路径 | `/open-api/v1/providers/{provider_name}` |
| 说明 | 删除前检查是否有 Cluster 或 Model-Price 引用 |

### 11.2 测试用例

| 用例编号 | 用例名称 | 预期结果 |
|----------|----------|----------|
| PV-5-001 | 删除 Provider | 200，删除后查询返回 404 |
| PV-5-002 | 删除不存在的 Provider | 404 |
| PV-5-003 | 删除被 Cluster 引用的 Provider | 409 |
| PV-5-004 | 删除被 ModelPrice 引用的 Provider | 200（当前实现允许级联或忽略该引用） |

### 11.3 测试场景详细设计

#### 11.3.1 PV-5-001：删除 Provider

##### 设计思路

验证正常删除 Provider 后，再次查询返回 404。

##### 前提数据准备

1. 创建 Provider。

##### 执行步骤

1. 发送 DELETE 请求到 `/open-api/v1/providers/{provider_name}`。
2. 验证响应 `ErrNum = 200`。
3. 发送 GET 请求查询该 Provider。
4. 验证响应 `ErrNum = 404`。

##### 预期返回结果

**DELETE ErrNum**：200  
**GET ErrNum**：404

---

#### 11.3.2 PV-5-002：删除不存在的 Provider

##### 设计思路

验证删除不存在的 Provider 返回 404。

##### 执行步骤

1. 发送 DELETE 请求到 `/open-api/v1/providers/non_existent_provider`。
2. 验证响应 `ErrNum = 404`。

##### 预期返回结果

**ErrNum**：404

---

#### 11.3.3 PV-5-003：删除被 Cluster 引用的 Provider

##### 设计思路

验证存在 Cluster 引用时，删除 Provider 被拒绝并返回 409。

##### 前提数据准备

1. 创建 Provider。
2. 创建 Cluster，通过 `llm_config.provider` 引用该 Provider。

##### 执行步骤

1. 发送 DELETE 请求。
2. 验证响应 `ErrNum = 409`。

##### 预期返回结果

**ErrNum**：409

---

#### 11.3.4 PV-5-004：删除被 ModelPrice 引用的 Provider

##### 设计思路

验证存在 Model-Price 引用时，当前实现允许删除 Provider（或该引用不影响删除），删除后查询返回 404。

##### 前提数据准备

1. 创建 Provider。
2. 创建 Model-Price，通过 `provider` 字段引用该 Provider。

##### 执行步骤

1. 发送 DELETE 请求。
2. 验证响应 `ErrNum = 200`。
3. 查询 Provider，验证返回 404。

##### 预期返回结果

**DELETE ErrNum**：200  
**GET ErrNum**：404


## 12. 触发模型发现（PV-6）

### 12.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Provider |
| 接口名称 | 触发模型发现 |
| 方法 | POST |
| 路径 | `/open-api/v1/providers/tools/discover-models` |
| 说明 | 无状态工具接口，根据协议向指定端点拉取模型列表 |

### 12.2 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `model_protocol` | string | Y | 模型协议，枚举：`openai`、`anthropic`、`gemini` |
| `schema` | string | Y | 协议方案，如 `http`、`https` |
| `addr` | string | Y | 服务端地址 |
| `port` | int | Y | 服务端端口 |
| `uri` | string | N | 发现端点路径，为空时按协议取默认值：`openai`/`anthropic` 为 `/v1/models`，`gemini` 为 `/v1beta/models` |
| `apikey` | string | N | 访问远端时使用的 API-Key |

### 12.3 测试用例

| 用例编号 | 用例名称 | 预期结果 |
|----------|----------|----------|
| PV-6-001 | OpenAI 协议模型发现 | 200，返回模型列表 `["m1", "m2"]` |
| PV-6-002 | Anthropic 协议模型发现 | 200，返回模型列表 `["claude-3-opus-20240229"]` |
| PV-6-002b | Gemini 协议模型发现 | 200，返回模型列表 `["gemini-2.5-pro", "gemini-2.5-flash"]`（`models[].name` 剥离 `models/` 前缀） |
| PV-6-003 | URI 为空时默认使用 /v1/models | 200，返回 `["m1"]` |
| PV-6-004 | 缺少必填参数 | 422 |
| PV-6-005 | 非法 model_protocol | 422 |
| PV-6-006 | 不带 apikey | 200，返回 `["m1"]` |

### 12.4 测试场景详细设计

#### 12.4.1 PV-6-001：OpenAI 协议模型发现

##### 设计思路

验证 OpenAI 协议下，工具接口能正确解析 `/v1/models` 返回的 `data[].id` 字段并返回模型 ID 列表。

##### 前提数据准备

1. 启动本地 HTTP 测试服务器，返回 `{"data":[{"id":"m1"},{"id":"m2"}]}`。
2. 解析得到服务器 host 与 port。

##### 执行步骤

1. 发送 POST 请求，携带 `model_protocol=openai`、`schema=http`、`addr`、`port`、`uri=/v1/models`、`apikey=sk-xxx`。
2. 验证响应 `ErrNum = 200`。
3. 断言返回的 `models` 为 `["m1", "m2"]`。

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| models | ["m1", "m2"] | Equals |

---

#### 12.4.2 PV-6-002：Anthropic 协议模型发现

##### 设计思路

验证 Anthropic 协议下，工具接口能正确解析 `models[].model_id` 字段。

##### 前提数据准备

1. 启动本地 HTTP 测试服务器，返回 `{"models":[{"model_id":"claude-3-opus-20240229","display_name":"Claude 3 Opus"}]}`。

##### 执行步骤

1. 发送 POST 请求，`model_protocol=anthropic`。
2. 验证响应 `ErrNum = 200`。
3. 断言返回的 `models` 为 `["claude-3-opus-20240229"]`。

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| models | ["claude-3-opus-20240229"] | Equals |

---

#### 12.4.3 PV-6-002b：Gemini 协议模型发现

##### 设计思路

验证 Gemini 协议下，工具接口默认调用 `/v1beta/models`，并能正确解析 `models[].name` 字段、剥离 `models/` 前缀。

##### 前提数据准备

1. 启动本地 HTTP 测试服务器，返回 `{"models":[{"name":"models/gemini-2.5-pro"},{"name":"models/gemini-2.5-flash"}]}`。

##### 执行步骤

1. 发送 POST 请求，`model_protocol=gemini`，不传 `uri`。
2. 验证响应 `ErrNum = 200`。
3. 断言返回的 `models` 为 `["gemini-2.5-pro", "gemini-2.5-flash"]`。

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| models | ["gemini-2.5-pro", "gemini-2.5-flash"] | Equals |

---

#### 12.4.4 PV-6-003：URI 为空时默认使用 /v1/models

##### 设计思路

验证请求体未传 `uri` 时，系统使用默认值 `/v1/models` 发起发现请求。

##### 执行步骤

1. 发送 POST 请求，不包含 `uri` 字段。
2. 验证响应 `ErrNum = 200`。
3. 断言返回模型列表。

##### 预期返回结果

**ErrNum**：200

---

#### 12.4.5 PV-6-004 ~ PV-6-006：参数与协议校验

##### 设计思路

覆盖必填参数缺失、非法协议、apikey 可选等场景。

| 用例编号 | 异常点 | 预期结果 |
|----------|--------|----------|
| PV-6-004 | 缺少 `addr` | 422 |
| PV-6-005 | `model_protocol=unknown` | 422 |
| PV-6-006 | 不传 `apikey` | 200 |

## 13. 获取所有 Provider 名称（PV-7）

### 13.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Provider |
| 接口名称 | 获取所有 Provider 名称 |
| 方法 | GET |
| 路径 | `/open-api/v1/providers/actions/get-provider-names` |
| 说明 | 返回全量 provider 名称列表 |

### 13.2 测试用例

| 用例编号 | 用例名称 | 预期结果 |
|----------|----------|----------|
| PV-7-001 | 获取所有 Provider 名称列表 | 200，`names` 字段非空 |

### 13.3 测试场景详细设计

#### 13.3.1 PV-7-001：获取所有 Provider 名称列表

##### 设计思路

验证接口返回当前所有 Provider 的名称数组，字段为 `names`。

##### 前提数据准备

1. 创建至少 2 个 Provider。

##### 执行步骤

1. 发送 GET 请求到 `/open-api/v1/providers/actions/get-provider-names`。
2. 验证响应 `ErrNum = 200`。
3. 断言 `names` 字段非空且包含已创建的 Provider 名称。

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| names | 非空数组 | NotEmpty |

## 14. instance_pool 默认 name 生成

### 14.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Provider / Inner API |
| 接口名称 | instance_pool 移除 name 字段后 backend Name 生成 |
| 方法 | POST + GET |
| 路径 | `/open-api/v1/providers` + `/inner-api/v1/configs/gslb_data/cluster_table` |
| 说明 | 验证 instance_pool 不再包含 `name` 字段时，cluster_table 导出使用 `addr_port` 作为 backend Name |

### 14.2 测试用例

| 用例编号 | 用例名称 | 预期结果 |
|----------|----------|----------|
| PV-NAME-1-001 | instance_pool 无 name 时 backend Name 为 addr_port | cluster_table 中 backend Name = `10.0.0.100_8080` |

### 14.3 测试场景详细设计

#### 14.3.1 PV-NAME-1-001：instance_pool 无 name 时 backend Name 为 addr_port

##### 设计思路

验证 Provider 的 `instance_pool` 元素移除 `name` 字段后，创建/更新返回的实例对象同样不含 `name`；且导出到 `cluster_table` 时，backend Name 使用 `{addr}_{port}` 格式。

##### 前提数据准备

1. 创建 Provider，`instance_pool` 不包含 `name` 字段，指定 `addr=10.0.0.100`、`port=8080`。
2. 创建 Cluster 引用该 Provider。

##### 执行步骤

1. 创建 Provider，断言返回的 `instance_pool[0]` 不包含 `name` 字段。
2. 创建 Cluster。
3. 调用 `GET /inner-api/v1/configs/gslb_data/cluster_table`。
4. 断言对应 cluster 的 backend `Name` 为 `10.0.0.100_8080`，`Addr` 为 `10.0.0.100`。

##### 预期返回结果

**ErrNum**：200

**cluster_table 校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Name | "10.0.0.100_8080" | Equals |
| Addr | "10.0.0.100" | Equals |


## 15. 设置高峰/闲时模板（PV-8）

### 15.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Provider |
| 接口名称 | 设置高峰/闲时模板 |
| 方法 | PUT |
| 路径 | `/open-api/v1/providers/{provider_name}/pricing-tiers` |
| 说明 | 单独维护 provider 的 `time_zone` 和 `tiers`；支持 JSON / YAML / multipart 上传 |

### 15.2 请求参数

#### 15.2.1 URI 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `provider_name` | string | Y | Provider 名称，必须已存在 |

#### 15.2.2 Body 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `time_zone` | string | N | 时区，默认 `Asia/Shanghai`；须为合法 IANA 时区名 |
| `tiers` | []object | N | 时段 tier 定义列表，结构与创建 Provider 的 `tiers` 一致 |

### 15.3 测试用例

| 用例编号 | 用例名称 | 预期结果 |
|----------|----------|----------|
| PT-1-001 | JSON 设置高峰模板 | 200，返回的 `time_zone`/`tiers` 与输入一致 |
| PT-1-002 | text/yaml 设置高峰模板 | 200，返回的 `time_zone`/`tiers` 与输入一致 |
| PT-1-003 | multipart/form-data YAML 设置高峰模板 | 200，返回的 `time_zone`/`tiers` 与输入一致 |
| PT-1-004 | provider 不存在 | 404 |
| PT-1-005 | 非法 `time_zone` | 422 |
| PT-1-006 | 非法 tier name | 422 |
| PT-1-007 | 同一 tier 时间重叠 | 422 |
| PT-1-008 | `end <= start` | 422 |
| PT-1-009 | `weekdays` 越界 | 422 |
| PT-1-010 | GET provider 返回 tiers | 200，响应中包含 `time_zone`/`tiers` |
| PT-1-011 | 设置 pricing tiers 后 `protocol_paths` 保留 | 200，`protocol_paths` 与创建时一致（`UpdatePricingTiers` 手工构造 updateParam 不丢字段回归） |

### 15.4 测试场景详细设计

#### 15.4.1 PT-1-001：JSON 设置高峰模板

##### 设计思路

验证通过 JSON Body 设置 pricing-tiers 后，返回的 `time_zone` 与 `tiers` 与输入一致。

##### 前提数据准备

1. 创建 Provider。

##### 执行步骤

1. 发送 PUT 请求，Content-Type 为 JSON，携带 `time_zone=Asia/Shanghai` 与一个 `peak` tier（两个不重叠 time_range）。
2. 验证响应 `ErrNum = 200`。
3. 断言 `time_zone` 为 `Asia/Shanghai`，`tiers` 长度 1，首个 tier 的 `name` 为 `peak`。

##### 请求参数

```json
{
  "time_zone": "Asia/Shanghai",
  "tiers": [
    {
      "name": "peak",
      "time_ranges": [
        {"weekdays": [1, 2, 3, 4, 5], "start": "09:00", "end": "12:00"},
        {"weekdays": [1, 2, 3, 4, 5], "start": "14:00", "end": "18:00"}
      ]
    }
  ]
}
```

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| time_zone | "Asia/Shanghai" | Equals |
| tiers | 长度 1，name=peak | Len + Equals |

---

#### 15.4.2 PT-1-002：text/yaml 设置高峰模板

##### 设计思路

验证通过 `text/yaml` Body 设置 pricing-tiers 同样生效。

##### 前提数据准备

1. 创建 Provider。

##### 执行步骤

1. 构造 YAML 内容，包含 `time_zone` 与 `tiers`。
2. 使用 `testutil.UpdatePricingTiersYAML` 发送 PUT 请求，Content-Type 为 `text/yaml`。
3. 验证响应 `ErrNum = 200`。
4. 断言 `time_zone` 与 tier 名称正确。

##### 请求参数（YAML）

```yaml
time_zone: "Asia/Shanghai"
tiers:
  - name: "peak"
    time_ranges:
      - weekdays: [1, 2, 3, 4, 5]
        start: "09:00"
        end: "12:00"
      - weekdays: [1, 2, 3, 4, 5]
        start: "14:00"
        end: "18:00"
```

##### 预期返回结果

**ErrNum**：200

---

#### 15.4.3 PT-1-003：multipart/form-data YAML 设置高峰模板

##### 设计思路

验证通过 multipart 文件上传 YAML 设置 pricing-tiers 同样生效。

##### 执行步骤

1. 构造 YAML 字节数组。
2. 使用 `testutil.UpdatePricingTiersMultipartYAML` 上传文件。
3. 验证响应 `ErrNum = 200`。
4. 断言 `time_zone` 正确。

##### 预期返回结果

**ErrNum**：200

---

#### 15.4.4 PT-1-004 ~ PT-1-009：参数校验异常场景

##### 设计思路

统一覆盖 pricing-tiers 接口的字段级校验。

| 用例编号 | 异常点 | 请求体关键特征 |
|----------|--------|----------------|
| PT-1-004 | Provider 不存在 | 使用未创建的 provider name |
| PT-1-005 | 非法 `time_zone` | `time_zone: "Mars/Phobos"` |
| PT-1-006 | 非法 tier name | `name: "off_peak"` |
| PT-1-007 | 同一 tier 时间重叠 | 两个 time_range 有交集 |
| PT-1-008 | `end <= start` | `start: "12:00", end: "09:00"` |
| PT-1-009 | `weekdays` 越界 | `weekdays: [7]` |

##### 预期返回结果

**ErrNum**：404（PT-1-004）或 422（PT-1-005 ~ PT-1-009）

---

#### 15.4.5 PT-1-010：GET provider 返回 tiers

##### 设计思路

验证设置 pricing-tiers 后，通过 GET Provider 详情可读取到 `time_zone` 与 `tiers`。

##### 前提数据准备

1. 创建 Provider。
2. 调用 pricing-tiers 接口设置 tiers。

##### 执行步骤

1. 发送 GET 请求到 `/open-api/v1/providers/{provider_name}`。
2. 验证响应 `ErrNum = 200`。
3. 断言返回的 `time_zone` 与 `tiers` 非空且与设置一致。

##### 预期返回结果

**ErrNum**：200

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| time_zone | 与设置一致 | Equals |
| tiers | 长度 >= 1 | Len + NotEmpty |

## 16. 依赖与数据准备

1. 测试二进制 `ai-gateway-api.exe` 已由 `make build` 生成。
2. `testutil.StartServer()` 启动完整服务，初始化 SQLite 与 miniredis。
3. 测试环境数据库需包含产品线初始化数据，以支持 cluster 创建。
4. 创建 Provider 后如需验证删除/更新冲突，需先创建引用该 Provider 的 Cluster 或 Model-Price。
5. 测试环境 `SkipTokenValidate=true`，无需认证头。

## 17. 注意事项

1. Provider 名称全局唯一，测试中使用 `testutil.UniqueProviderName()` 生成唯一名称。
2. `keys` 作为数组，按**全量替换**处理；更新时如需保留旧 Key，需传入完整列表。
3. `models` 同样按**全量替换**处理；若移除被 Cluster 引用的 Model，更新会被拒绝（409）。
4. `instance_pool` 中 `(addr, port)` 组合不能重复；`name` 为空字符串时系统会默认填充为 `addr`。
5. `model_protocols` 当前枚举值为 `openai`、`anthropic`、`gemini`。
6. `/providers/tools/discover-models` 为无状态工具接口，不绑定 Provider；集成测试通过本地 `httptest` 服务器模拟远端响应。
7. `PUT /providers/{provider_name}/pricing-tiers` 只更新 `time_zone`/`tiers`，不会覆盖 provider 的其他字段。
8. 更新 Provider 时若请求体包含 `name`，接口返回 422；`name` 只能通过创建接口指定。

## 18. K8s 实例池（/k8s_pools）

### 18.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Inner API / Provider |
| 接口名称 | K8s 实例池维护（k8s-pools.md） |
| 方法 | PUT/GET/DELETE |
| 路径 | `/inner-api/v1/k8s_pools/{name}/instances`、`/inner-api/v1/k8s_pools/{name}`、`/inner-api/v1/k8s_pools` |
| 说明 | 发现组件唯一写入通道；`k8s_pools` 是 k8s 来源实例的唯一事实源，provider 只保存 `k8s_pool_name` 引用，导出/batch 消费时实时解析（无镜像、无 fan-out） |

### 18.2 测试用例

| 用例编号 | 用例名称 | 预期结果 |
|----------|----------|----------|
| KP-1-001 | 端到端全链路（bug 回归）：PUT pool → 建 k8s_pool provider → 建 cluster（**不再 PUT pool**）→ 导出立即含实例 → 再 PUT 增删实例 → 导出跟随 → DELETE 被引用 pool → 保留行且 `instance_count=0`、导出空条目 | 全链路 200/404 断言成立 |
| KP-1-002 | 顺序反转：先建 provider/cluster（空）→ 首次 PUT pool → 导出立即跟随（无 fan-out 回填） | 导出出现 backend |
| KP-1-003 | cluster PATCH 不清空：k8s provider + 非空 pool → PATCH cluster → 导出仍有实例 | backend 保留 |
| KP-1-004 | provider 模式切换：instance_pool → k8s_pool 导出切到 pool；k8s_pool → instance_pool 导出切回手工池 | 导出前后形态正确 |
| KP-2-001 | 查询全部 pool 列表 | 200，`list` 含条目及 `instance_count`/`last_sync_time` |
| KP-3-001 | 参数校验：非法 pool 名、重复 addr/port、weight/port 越界、缺 addr | 422 |
| KP-3-002 | 查询/删除不存在的 pool | 404 |
| KP-4-001 | OpenAPI：请求携带已删除的 `k8s_instance_pool` 被静默忽略（provider 正常创建、响应无该字段）、非法 instance_source 422、k8s_pool 模式缺 k8s_pool_name 422 | 200 无镜像字段；422 |
| KP-4-002 | 模式切回 instance_pool 恢复休眠 instance_pool 与 k8s_pool_name；响应不含 `k8s_instance_pool` | 200，字段休眠保留 |
| KP-5-001 | 删除未被引用 pool | 200，GET 404 |
| KP-5-002 | 删除被引用 pool（保留行 + 清空实例） | 200，GET 200 + `instance_count=0`，导出空条目 |
| KP-6-001 | 引用不存在的 pool（池不存在 ≡ 零实例） | 导出空条目，不报错 |
| KP-6-002 | PUT 空实例列表（不删 pool） | 200，`instance_count=0`，导出空条目 |
| KP-6-003 | 版本 bump：pool PUT 前后 cluster_table `Version` 必须变化 | Version 变化 |
| KP-7-001 | 4xx 回读零变更：PATCH 非法 k8s_pool_name → 422 → provider 的 k8s_pool_name 与 instance_pool 逐字段不变 | 422，读回与操作前一致 |
| KP-7-002 | 已删除字段静默忽略：创建携带 `k8s_instance_pool` → 200 创建成功且响应无该字段 | 200 无镜像字段 |
| KP-7-003 | 4xx 回读零变更：PATCH 切 k8s_pool 模式缺 k8s_pool_name → 422 → instance_source 不变 | 422，读回与操作前一致 |
| KP-7-004 | 4xx 回读零变更 + 防泄漏：已存在 pool 上非法 PUT → 422 → pool、cluster_table 导出逐字段不变（非法实例不得出现在导出产物） | 422，读回不变 |
| KP-7-005 | PATCH 省略矩阵（k8s 模式）：PATCH 只改 description（合同必填字段照常携带）→ models/keys/休眠 instance_pool/k8s_pool_name 均未提交字段原值不变 | 200，未提交字段全保留 |
| KP-7-006 | 省略 vs 显式空对照：切回 instance_pool 时显式 `instance_pool: []` → 422 且休眠池原值保留；携带休眠池原值提交 → 200 且休眠池恢复生效、k8s_pool_name 休眠保留 | 前者 422，后者 200 |
| KP-7-007 | 审计：模式切换 PATCH 产生 provider update 日志，`diff_keys` 精确等于实际变更字段（ElementsMatch，禁止 Contains） | diff_keys = [instance_source, k8s_pool_name] |

> 说明：k8s 模式不再维护 provider 镜像（`k8s_instance_pool` 字段已删除），provider 响应只含 `k8s_pool_name` 引用；`cluster_table` 在每次导出时按 `k8s_pool_name` 实时查询 `k8s_pools`，池不存在等价于零实例。
>
> 手动来源（`instance_source=instance_pool`）的实例池同步链路与用例保持原样；manual → k8s 模式切换会清空派生池中的旧手工快照（占位不变量）。
