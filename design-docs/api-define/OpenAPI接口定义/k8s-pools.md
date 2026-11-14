# /k8s_pools

## 1. 概述

K8s 实例池（`k8s_pools`）是控制面内的只读资源视图。池成员由 K8s 发现组件（service-controller）通过 InnerAPI `/inner-api/v1/k8s_pools/{name}/instances` 全量上报，OpenAPI 不提供写入/删除入口。

本模块用于：

1. provider 配置为 `instance_source=k8s_pool` 时，展示当前可选的 K8s pool 列表；
2. provider 详情页按 `k8s_pool_name` 展示该 pool 当前可用的实例列表、实例数量和最近同步时间。

> 实际 URL 前缀为 `/open-api/v1`，即列表接口为 `GET /open-api/v1/k8s_pools`，详情接口为 `GET /open-api/v1/k8s_pools/{name}`。

## 2. 接口清单

| 序号 | 端点 | Method | 功能描述 |
|------|------|--------|----------|
| 1 | `/k8s_pools` | GET | 查询全部 K8s pool 摘要列表 |
| 2 | `/k8s_pools/{name}` | GET | 查询单个 K8s pool 及其实例列表 |

**鉴权**

| 项目 | 值 |
|------|------|
| 通用鉴权 | Token（`Authorization` Header，见 [00-common.md](./00-common.md)） |
| 端点授权 | `FeatureK8sPool` + `ActionRead` |

**只读边界**

- OpenAPI 仅提供上述两个 `GET` 接口。
- K8s pool 的 `PUT` / `DELETE` 仍只保留在 InnerAPI，供 K8s 发现组件使用；控制台不能通过 OpenAPI 修改池成员。

---

### 2.1 查询全部 K8s pool 列表

**基本信息**

| 项目 | 值 | 说明 |
|------|------|------|
| 含义 | 查询全部已存在的 K8s pool 摘要 | 供 provider 配置时选择 `k8s_pool_name` |
| 端点 | `/k8s_pools` | - |
| 版本 | v1 | - |
| Method | GET | - |
| 是否分页 | 否 | 本期返回全部 pool，与 InnerAPI 一致 |

**输入参数**

无 URI / Query / Body 参数。

**执行逻辑**

1. 查询 `k8s_pools` 表中的全部 pool。
2. 按 pool 的 `name`、实例数量、最近同步时间组装摘要列表。
3. 返回列表；无 pool 时返回空数组，不返回 404。

**返回数据（Data内容）**

| 字段 | 类型 | 说明 |
|------|------|------|
| `list` | []object | pool 摘要列表 |
| `list[].name` | string | pool 名称 |
| `list[].instance_count` | int | 当前实例数量；空池为 0 |
| `list[].last_sync_time` | int64 | 最近同步时间（Unix 秒），取 `k8s_pools.updated_at` |

**成功返回示例**

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

空列表示例：

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "list": []
    }
}
```

**错误码**

| ErrNum | 场景 |
|--------|------|
| 200 | 成功 |
| 401 | 鉴权失败 |
| 402 | 没有 `FeatureK8sPool` 的读权限 |
| 403 | 管理面 IP 白名单拒绝（全局中间件，可能发生） |
| 500 | 数据库或其他业务错误 |

---

### 2.2 查询单个 K8s pool 条目

**基本信息**

| 项目 | 值 | 说明 |
|------|------|------|
| 含义 | 查询指定 pool 的实例列表与同步状态 | provider 详情页按 `k8s_pool_name` 展示实例 |
| 端点 | `/k8s_pools/{name}` | - |
| 版本 | v1 | - |
| Method | GET | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
|--------|------|----------|------|----------|------------|
| `name` | string | K8s pool 名称 | Y | 与 provider 的 `k8s_pool_name` 对应 | 必填；长度 1-128 字符；仅允许字母、数字、`_`、`-`、`.`；不能以 `.`、`-`、`_` 开头或结尾；不能包含空白字符 |

**执行逻辑**

1. 校验 URI 中的 `name`。
2. 查询 `k8s_pools` 表中对应条目。
3. 条目不存在时返回 404；存在时返回实例列表、实例数量和最近同步时间。
4. 空池（`instances=[]`）是合法状态，返回 200 而非 404。

**返回数据（Data内容）**

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | pool 名称 |
| `instances` | []Instance | 当前实例列表；空池为 `[]` |
| `instance_count` | int | 实例数量，等于 `len(instances)` |
| `last_sync_time` | int64 | 最近同步时间（Unix 秒），取 `k8s_pools.updated_at` |

**Instance 结构（`instances` 元素）**

| 字段 | 类型 | 说明 | 可能取值 | 合法性条件 |
|------|------|------|----------|------------|
| `addr` | string | 实例地址 | DNS 名或 IP | 类型为 [Hostname](./00-common.md#1-主机名hostname) |
| `port` | int | 实例端口 | 1-65535 | 类型为 [Port](./00-common.md#3-网络端口port) |
| `weight` | int | 实例权重 | 0-100 | 范围 [0,100]；发现组件未上报时默认 100 |
| `disable` | bool | 实例是否禁用（系统只读） | `true` / `false` | 只读；当前发现组件只上报 Ready endpoint，通常为 `false`；OpenAPI 不接受写入 |

> `disable` 来自控制面内部的实例结构，InnerAPI 的实例响应同样包含该字段。OpenAPI 将其作为只读系统字段返回，前端可忽略。

**成功返回示例**

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

**错误码**

| ErrNum | 场景 |
|--------|------|
| 200 | 成功 |
| 401 | 鉴权失败 |
| 402 | 没有 `FeatureK8sPool` 的读权限 |
| 403 | 管理面 IP 白名单拒绝（全局中间件，可能发生） |
| 404 | pool 不存在 |
| 422 | `name` 参数不合法 |
| 500 | 数据库或其他业务错误 |

---

## 3. 与 provider 的关系

provider 在 `instance_source=k8s_pool` 模式下通过 `k8s_pool_name` 引用本模块的 pool：

1. provider 配置时：
   - 调用 `GET /open-api/v1/k8s_pools` 展示可选池；
   - 将选中的 `name` 写入 provider 的 `k8s_pool_name`。
2. provider 详情页：
   - 从 `GET /open-api/v1/providers/{provider_name}` 读取 `instance_source` 和 `k8s_pool_name`；
   - 当 `instance_source == "k8s_pool"` 时，调用 `GET /open-api/v1/k8s_pools/{k8s_pool_name}` 展示实时实例列表。

本模块只提供池的只读视图，不修改 provider 的请求/响应契约。

---

## 4. 与 InnerAPI 的关系

| 能力 | OpenAPI | InnerAPI |
|------|---------|----------|
| 查询 pool 列表 | `GET /open-api/v1/k8s_pools` | `GET /inner-api/v1/k8s_pools` |
| 查询单个 pool | `GET /open-api/v1/k8s_pools/{name}` | `GET /inner-api/v1/k8s_pools/{name}` |
| 全量替换实例 | 不提供 | `PUT /inner-api/v1/k8s_pools/{name}/instances` |
| 删除 pool | 不提供 | `DELETE /inner-api/v1/k8s_pools/{name}` |

两个域的读取数据同源，均来自 `k8s_pools` 表；OpenAPI 仅改变鉴权、路由和对外契约的分层，不产生第二份实例数据。

> InnerAPI 的详细定义见 [k8s-pools.md](../InnerAPI接口定义/k8s-pools.md)。

---

## 5. 约束与说明

- OpenAPI 不开放 K8s pool 的写入/删除能力，池成员唯一写入方是 K8s 发现组件。
- `last_sync_time` 为 Unix 秒；前端可用于提示池数据的新鲜度。
- 列表接口不分页；当前池数量受 K8s Service 数量约束，后续如有规模问题再以兼容方式扩展分页。
- pool 不存在与空池语义不同：
  - 不存在：404；
  - 存在但零实例：200 + `instances=[]`。
- 响应包含后端实例地址，仅对具备 `FeatureK8sPool` 读权限的用户开放。