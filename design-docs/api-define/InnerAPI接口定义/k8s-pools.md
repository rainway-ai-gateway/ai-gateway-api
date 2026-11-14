# k8s_pools 接口

K8s 实例池（`k8s_pools`）是控制面内独立的实例集合资源，成员列表的**唯一写入方是 K8s 发现组件**（service-controller）。`k8s_pools` 是 k8s 来源实例的**唯一事实源**：引用该池的 provider（`instance_source=k8s_pool`）只保存 `k8s_pool_name` 引用，不存储任何实例数据；人通过 OpenAPI `/providers` 维护池引用（`instance_source` / `k8s_pool_name`），不触碰池成员。导出（`cluster_table`）与 batch 出网在消费时按 `k8s_pool_name` 实时查询本表，不落任何实例镜像、不做 fan-out。

与导出类接口不同，本域为资源读写接口（PUT/GET/DELETE），不支持 `version` 增量同步参数。

## 1. 接口信息

| 项目 | 值 | 说明 |
|------|------|------|
| 含义 | K8s 实例池成员维护与查询 | 供发现组件写入发现的实例快照；控制台/运维只读查询同步状态 |
| 端点前缀 | `/k8s_pools` | - |
| 鉴权 | router 级 McUserProbe + 端点级 feature/action 授权 | k8s_pools 域独立授权项；GET=Read、PUT=Update、DELETE=Delete |

| 序号 | 端点 | Method | 功能描述 |
|------|------|--------|----------|
| 1 | `/inner-api/v1/k8s_pools/{name}/instances` | PUT | 全量替换 pool 实例列表（幂等 upsert） |
| 2 | `/inner-api/v1/k8s_pools/{name}` | GET | 查询单个 pool 条目 |
| 3 | `/inner-api/v1/k8s_pools` | GET | 查询全部 pool 列表 |
| 4 | `/inner-api/v1/k8s_pools/{name}` | DELETE | 删除 pool（无引用删行；被引用时清空保留） |

## 2. 全量替换实例列表

**基本信息**

| 项目 | 值 | 说明 |
|------|------|------|
| 含义 | 全量替换指定 pool 的实例列表（**只写 `k8s_pools` 单表**） | 条目不存在则创建（幂等 upsert）；可任意重试；不写 provider / cluster / pools 等任何其它表，消费方下一次读取时实时反映 |
| 端点 | `/inner-api/v1/k8s_pools/{name}/instances` | - |
| Method | PUT | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 合法性条件 |
|--------|------|----------|------|------------|
| name | string | pool 名称 | Y | 长度 1-128 字符；仅允许字母、数字、`_`、`-`、`.`；不能以 `.`、`-`、`_` 开头或结尾；不能包含空白字符；与 K8s Service 名无耦合（Service→池名映射由发现组件自行约定） |

**输入参数（Body）**

Instance 数组（全量快照，空数组合法，表示"零实例"）：

| 参数名 | 类型 | 参数含义 | 必填 | 合法性条件 |
|--------|------|----------|------|------------|
| addr | string | 实例地址 | Y | 类型为 [Hostname](../OpenAPI接口定义/00-common.md#1-主机名hostname) |
| port | int | 实例端口 | Y | 类型为 [Port](../OpenAPI接口定义/00-common.md#3-网络端口port) |
| weight | int | 实例权重 | N | 取值范围 [0,100]；默认 `100` |

同一 pool 内 `(addr, port)` 组合不能重复。

**请求示例**

```shell
curl -X PUT "http://api-server:port/inner-api/v1/k8s_pools/svc-a/instances" \
  -H "Authorization:Token TOKEN_STRING" \
  -H "Content-Type: application/json" \
  -d '[
    {"addr": "10.0.0.1", "port": 8000, "weight": 100},
    {"addr": "10.0.0.2", "port": 8000, "weight": 100}
  ]'
```

**返回数据（Data内容）**

更新后的 pool 条目，结构同 §3 查询单个 pool 条目。

## 3. 查询单个 pool 条目

**基本信息**

| 项目 | 值 | 说明 |
|------|------|------|
| 含义 | 查询指定 pool 的条目（实例列表 + 同步状态） | - |
| 端点 | `/inner-api/v1/k8s_pools/{name}` | - |
| Method | GET | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 合法性条件 |
|--------|------|----------|------|------------|
| name | string | pool 名称 | Y | 同 §2 |

**返回数据（Data内容）**

| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | pool 名称 |
| instances | []Instance | 当前实例列表（结构同 §2 Body 元素，另含系统只读字段 `disable`） |
| instance_count | int | 实例数量 |
| last_sync_time | int64 | 最近同步时间（unix 秒），取条目 `updated_at` |

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "name": "svc-a",
        "instances": [
            {"addr": "10.0.0.1", "port": 8000, "weight": 100, "disable": false},
            {"addr": "10.0.0.2", "port": 8000, "weight": 100, "disable": false}
        ],
        "instance_count": 2,
        "last_sync_time": 1716883200
    },
    "WorkMode": "ModeNormal"
}
```

pool 不存在时返回 404。

## 4. 查询全部 pool 列表

**基本信息**

| 项目 | 值 | 说明 |
|------|------|------|
| 含义 | 查询全部已存在的 pool 及各 pool 的实例数与最近同步时间 | - |
| 端点 | `/inner-api/v1/k8s_pools` | - |
| Method | GET | - |

**输入参数**

无。

**返回数据（Data内容）**

| 字段 | 类型 | 说明 |
|------|------|------|
| list | []object | pool 摘要列表，元素含 `name`、`instance_count`、`last_sync_time`（含义同 §3） |

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "list": [
            {"name": "svc-a", "instance_count": 3, "last_sync_time": 1716883200},
            {"name": "svc-b", "instance_count": 0, "last_sync_time": 1716883200}
        ]
    },
    "WorkMode": "ModeNormal"
}
```

## 5. 删除 pool

**基本信息**

| 项目 | 值 | 说明 |
|------|------|------|
| 含义 | 删除指定 pool（**只写 `k8s_pools` 单表**） | 无 provider 引用时删除行；被 provider 引用时保留行并清空 `instances`（warning 日志），保留 `last_sync_time` 便于排障 |
| 端点 | `/inner-api/v1/k8s_pools/{name}` | - |
| Method | DELETE | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 合法性条件 |
|--------|------|----------|------|------------|
| name | string | pool 名称 | Y | 同 §2 |

**执行逻辑**

1. 查询 `k8s_pools` 行（不存在 → 404）。
2. 只读查询引用该 pool 的 provider（`instance_source=k8s_pool && k8s_pool_name==name`）判断引用情况：
   - **无引用**：删除 `k8s_pools` 行；
   - **有引用**：保留行，将 `instances` 清空（warning 日志）。
3. 上述两步在同一事务内完成；除读取 provider 引用外不写任何其它表，也不触发 cluster 派生池同步。

保留两种语义的理由：现有 OpenAPI/IoT 契约区分"pool 不存在（404）"与"pool 存在但为空（200 + []）"；对消费者而言两种语义都等价于零实例。

provider 视角下"池不存在"与"池存在但零实例"是同一状态（有效池均为空）。

**返回数据（Data内容）**

Data 为 null。pool 不存在时返回 404。

## 6. 写入事务语义（单表）

§2 的 PUT 与 §5 的 DELETE 均为**单表写**，在单个数据库事务内只操作 `k8s_pools`：

1. 写 `k8s_pools` 行（PUT 更新 instances / DELETE 删行或清空）；
2. DELETE 时只读查询引用该 pool 的 provider 以决定"删行"或"清空保留"（对 provider 表不写）。

不写 `providers` / `clusters` / `pools` / `sub_clusters` 任何表，不维护实例镜像，不触发 cluster 派生池 fan-out。消费方（`cluster_table` 导出、batch 出网）在下一次请求时实时读取本表；导出结果的版本号由导出签名机制随实例变化自动 bump。多个 provider 引用同一 pool（N:1）时共享同一份实例列表，无额外成本。

## 7. 错误码

| ErrNum | 场景 |
|--------|------|
| 200 | 成功 |
| 401 | 鉴权失败 |
| 402 | 没有调用权限（feature/action 授权不足） |
| 404 | GET/DELETE 的 pool 不存在 |
| 422 | 参数不合法（pool 名称非法、body 非合法 Instance 数组、`(addr, port)` 重复、`weight` 越界等） |
| 500 | 其他业务逻辑错误（含单表事务回滚） |
