# OpenAPI K8s 实例池只读接口：API 变更说明

> 本文档描述 `ai-gateway-api` 为 provider 配置与详情展示新增的 K8s 实例池 OpenAPI 只读接口。设计见本目录 `design-changes.md`；摘要见本目录 `change-summary.md`。

## 1. 变更范围

| 接口 | 类型 | 变更 |
|------|------|------|
| `GET /open-api/v1/k8s_pools` | OpenAPI | 新增：查询全部 K8s pool 摘要 |
| `GET /open-api/v1/k8s_pools/{name}` | OpenAPI | 新增：查询单个 K8s pool 及其实例列表 |
| `GET /inner-api/v1/k8s_pools` | InnerAPI | 不变 |
| `GET /inner-api/v1/k8s_pools/{name}` | InnerAPI | 不变 |
| `PUT /inner-api/v1/k8s_pools/{name}/instances` | InnerAPI | 不变 |
| `DELETE /inner-api/v1/k8s_pools/{name}` | InnerAPI | 不变 |

无修改端点、无删除端点、无字段重命名。OpenAPI 仅新增只读能力，不开放 K8s pool 写入/删除。

---

## 2. OpenAPI：查询全部 K8s pool 列表

### 2.1 基本信息

| 项目 | 值 | 说明 |
|------|------|------|
| 含义 | 查询全部已存在的 K8s pool 摘要 | 供 provider 配置时选择 `k8s_pool_name` |
| 外部端点 | `GET /open-api/v1/k8s_pools` | - |
| 代码端点 | `Endpoint.Path = "/k8s_pools"` | OpenAPI router 统一挂载 `/open-api/v1` 前缀 |
| 鉴权 | `FeatureK8sPool` + `ActionRead` | 与 InnerAPI 读接口一致 |
| 是否分页 | 否 | 本期与 InnerAPI 一致，返回全部 pool |

### 2.2 输入参数

无 URI / Query / Body 参数。

> 说明：不引入 `page` / `page_size`。provider 下拉需要完整列表，且 pool 数量受 K8s Service 数量约束；后续如有规模问题再以兼容方式扩展分页。

### 2.3 返回数据（Data）

| 字段 | 类型 | 说明 |
|------|------|------|
| `list` | []object | pool 摘要列表 |
| `list[].name` | string | pool 名称 |
| `list[].instance_count` | int | 当前实例数量 |
| `list[].last_sync_time` | int64 | 最近同步时间（Unix 秒） |

顺序：沿用 `ik8s_pool` storage 的现有顺序（`k8s_pools.id` 升序），与 InnerAPI 列表一致。

### 2.4 成功返回示例

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "list": [
            {
                "name": "svc-a",
                "instance_count": 3,
                "last_sync_time": 1716883200
            },
            {
                "name": "svc-b",
                "instance_count": 0,
                "last_sync_time": 1716883200
            }
        ]
    }
}
```

- 无 pool 时返回 `{"list":[]}`，不是 404。
- 空池（`instance_count=0`）仍出现在列表中。

### 2.5 错误码

| ErrNum | 场景 |
|--------|------|
| 200 | 成功 |
| 401 | 鉴权失败 |
| 402 | 无 `FeatureK8sPool` Read 权限 |
| 403 | 管理面 IP 白名单拒绝（全局中间件，可能发生） |
| 500 | 数据库或其他业务错误 |

---

## 3. OpenAPI：查询单个 K8s pool 条目

### 3.1 基本信息

| 项目 | 值 | 说明 |
|------|------|------|
| 含义 | 查询指定 pool 的实例列表与同步状态 | 供 provider 详情页按 `k8s_pool_name` 展示实例 |
| 外部端点 | `GET /open-api/v1/k8s_pools/{name}` | - |
| 代码端点 | `Endpoint.Path = "/k8s_pools/{name}"` | OpenAPI router 统一挂载 `/open-api/v1` 前缀 |
| 鉴权 | `FeatureK8sPool` + `ActionRead` | 与 InnerAPI 读接口一致 |

### 3.2 输入参数（URI）

| 参数名 | 类型 | 必填 | 说明 | 合法性条件 |
|--------|------|------|------|------------|
| `name` | string | Y | pool 名称 | 长度 1-128 字符；仅允许字母、数字、`_`、`-`、`.`；不能以 `.`、`-`、`_` 开头或结尾；不能包含空白字符 |

复用 `iprovider.K8sPoolName` 校验；非法名称返回 422。

### 3.3 返回数据（Data）

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | pool 名称 |
| `instances` | []Instance | 当前实例列表；空池为 `[]` |
| `instance_count` | int | 实例数量，等于 `len(instances)` |
| `last_sync_time` | int64 | 最近同步时间（Unix 秒），取 `k8s_pools.updated_at` |

**Instance 结构**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `addr` | string | Y | 实例地址；类型为 [Hostname](../../api-define/OpenAPI接口定义/00-common.md#1-主机名hostname) |
| `port` | int | Y | 实例端口；类型为 [Port](../../api-define/OpenAPI接口定义/00-common.md#3-网络端口port) |
| `weight` | int | Y | 实例权重，范围 [0,100]；发现组件未上报时默认 100 |
| `disable` | bool | N | 系统字段，只读；当前发现组件只上报 Ready endpoint，通常为 `false` |

> `disable` 来自现有 `iprovider.ProviderInstance` 的序列化结果，InnerAPI 实际响应也包含该字段。本设计保留它以避免 OpenAPI 与 InnerAPI 实际响应不一致；OpenAPI 文档应将 `disable` 标注为系统只读字段。

### 3.4 成功返回示例

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "name": "svc-a",
        "instances": [
            {
                "addr": "10.0.0.1",
                "port": 8000,
                "weight": 100,
                "disable": false
            },
            {
                "addr": "10.0.0.2",
                "port": 8000,
                "weight": 100,
                "disable": false
            }
        ],
        "instance_count": 2,
        "last_sync_time": 1716883200
    }
}
```

空池示例：

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "name": "svc-empty",
        "instances": [],
        "instance_count": 0,
        "last_sync_time": 1716883200
    }
}
```

### 3.5 错误码

| ErrNum | 场景 |
|--------|------|
| 200 | 成功 |
| 401 | 鉴权失败 |
| 402 | 无 `FeatureK8sPool` Read 权限 |
| 403 | 管理面 IP 白名单拒绝（全局中间件，可能发生） |
| 404 | pool 不存在 |
| 422 | `name` 不合法 |
| 500 | 数据库或其他业务错误 |

池不存在与空池是两个不同状态：

- pool 不存在：404；
- pool 存在但零实例：200 + `instances=[]`。

---

## 4. 使用场景与调用关系

### 4.1 provider 配置时选择 K8s pool

```text
GET /open-api/v1/k8s_pools
    -> 前端展示 name / instance_count / last_sync_time
    -> 用户选择 k8s_pool_name
    -> POST/PATCH /open-api/v1/providers 提交 instance_source=k8s_pool + k8s_pool_name
```

### 4.2 provider 详情展示实例

```text
GET /open-api/v1/providers/{provider_name}
    -> 读取 instance_source / k8s_pool_name
    -> 若 instance_source == "k8s_pool"
       GET /open-api/v1/k8s_pools/{k8s_pool_name}
       -> 展示 instances / instance_count / last_sync_time
```

本变更不要求 provider 响应内嵌实例列表；前端通过 `k8s_pool_name` 调用本接口获取实时状态，避免维护第二份实例数据。

---

## 5. 契约文档变更

1. 新增 `design-docs/api-define/OpenAPI接口定义/k8s-pools.md`：
   - 模块说明、接口清单、两个 GET 接口的完整参数/响应/错误码/示例；
   - 明确 OpenAPI 不提供 PUT/DELETE；
   - 明确 `FeatureK8sPool` 读权限；
   - 明确 `disable` 为系统只读字段。
2. 更新 `design-docs/api-define/OpenAPI接口定义/README.md`：
   - 在模块接口表中增加 `/k8s_pools` 行，链接到新文档。
3. 更新 `design-docs/api-define/OpenAPI接口定义/providers.md`：
   - 在 k8s 模式响应示例后增加指引：
     > K8s pool 的实时实例列表可通过 `GET /open-api/v1/k8s_pools/{k8s_pool_name}` 查询。
   - 本变更不修改 provider 的字段和请求/响应结构。
4. 更新 `design-docs/api-define/InnerAPI接口定义/k8s-pools.md`：
   - 在实例结构说明中补充 `disable` 为响应中的系统只读字段；
   - 请求 schema 不变，`disable` 不参与写入校验。

---

## 6. 兼容性

- 纯新增端点，不影响现有 OpenAPI 路径、请求/响应和错误码。
- 不改变 InnerAPI `/k8s_pools` 的写入语义和 provider 同步链路。
- 不改变数据库 schema。
- 前端未接入时，既有 provider 配置/详情功能不受影响；接入后仅增加只读查询能力。

---

## 7. 与相关 issue 的关系

- `#290`（本变更）：OpenAPI 暴露 K8s pool 只读查询，解决 provider 配置选择池和详情展示实例。
- `#289`：provider 在 `k8s_instance_pool` 镜像为空时导致 cluster_table 实例为空的问题。本变更提供实时读取 `k8s_pools` 的 OpenAPI 通道，可独立落地；不依赖 `#289` 的修复，也不修改 provider 契约。