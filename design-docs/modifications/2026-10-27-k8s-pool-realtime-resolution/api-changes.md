# k8s_pool 实时解析改造（方案 B）：API 契约变更

> 摘要见本目录 `change-summary.md`；实现设计见本目录 `design-changes.md`。

## 1. 变更总览

| 接口 | 变化 |
|---|---|
| `GET /open-api/v1/providers/{name}` | 响应**移除 `k8s_instance_pool` 字段** |
| `GET /open-api/v1/providers` | 列表项**移除 `k8s_instance_pool` 字段** |
| `POST /open-api/v1/providers` | 字段已彻底删除：请求体携带 `k8s_instance_pool` **被静默忽略**（未知字段） |
| `PATCH /open-api/v1/providers/{name}` | 同上：请求体携带该字段被静默忽略 |
| `GET /inner-api/v1/configs/gslb_data/cluster_table` | 格式不变；k8s cluster 的实例来自 `k8s_pools` 实时解析（无 fan-out 延迟） |
| `PUT /inner-api/v1/k8s_pools/{name}/instances` | 路径/鉴权/请求/响应不变；**改为只写 `k8s_pools` 单表**（删除 provider 镜像刷新与派生池 fan-out） |
| `DELETE /inner-api/v1/k8s_pools/{name}` | 路径/鉴权/语义不变（无引用删行、有引用清空保留）；**改为单表写** |
| `GET /open-api/v1/k8s_pools`、`GET /open-api/v1/k8s_pools/{name}` | 不变（#290） |
| `GET /inner-api/v1/k8s_pools`、`GET /inner-api/v1/k8s_pools/{name}` | 不变 |

## 2. OpenAPI `/providers` 变更明细

### 2.1 响应移除 `k8s_instance_pool`

**改造前** 数据模型（节选）：

```json
{
    "instance_source": "k8s_pool",
    "k8s_pool_name": "svc-a",
    "instance_pool": [],
    "k8s_instance_pool": [
        {"addr": "10.0.0.1", "port": 8000, "weight": 100},
        {"addr": "10.0.0.2", "port": 8000, "weight": 100}
    ]
}
```

**改造后**：响应中不再出现 `k8s_instance_pool`。k8s 模式下的实时实例通过 `GET /open-api/v1/k8s_pools/{k8s_pool_name}` 查询（见 [k8s-pools.md](../../api-define/OpenAPI接口定义/k8s-pools.md)）：

```json
{
    "instance_source": "k8s_pool",
    "k8s_pool_name": "svc-a",
    "instance_pool": []
}
```

字段表删除 `k8s_instance_pool` 行（原定义："K8s 实例池只读镜像，系统从 `/k8s_pools` 同步，只读，请求体中携带返回 422"）。

### 2.2 有效池语义改写

**改造前**：

```
有效池 = instance_source == "k8s_pool" ? k8s_instance_pool : instance_pool
```

- `instance_source=k8s_pool`：有效池即系统维护的 `k8s_instance_pool` 镜像；`instance_pool` 休眠保留但不生效。
- 有效池内容变化时，系统自动同步更新引用该 provider 的所有 cluster 所生成的实例池。

**改造后**：

```
有效池 = instance_source == "k8s_pool" ? 实时查询 k8s_pools[k8s_pool_name].instances : instance_pool
```

- `instance_source=k8s_pool`：有效池 = 消费时按 `k8s_pool_name` 实时查询 `k8s_pools`（`k8s_pools` 是唯一事实源，provider 上不存实例数据）；`instance_pool` 休眠保留但不生效。
- 不再有任何"镜像同步"：PUT/DELETE `/k8s_pools` 只写 `k8s_pools` 单表，下一次导出/batch 即时反映；cluster 派生池在 k8s 模式下为空占位行，不参与实例数据承载。
- 池不存在 / 池名为空 / 池为空 ⇒ 有效池为空列表（零实例），不报错；引用 cluster 以空条目下发（`BK_NO_BACKEND`）。

### 2.3 请求体字段彻底删除

`k8s_instance_pool` 已从请求体定义与 DDL 彻底删除：请求携带该字段按未知字段**静默忽略**（binder 默认行为），响应同样不含该字段。

### 2.4 PATCH 语义更新

- 模式切换（`instance_pool` ↔ `k8s_pool`）时：manual → k8s 会清空引用 cluster 派生池中的旧手工实例快照（维持占位不变量）；k8s → manual 会写入新的 `instance_pool` 快照。
- `instance_source=k8s_pool` 时 PATCH `k8s_pool_name`（A→B）只改引用，不触发任何派生池同步，导出实时读 B。
- `instance_source=k8s_pool` 时 PATCH `instance_pool` 仅作休眠保留（不参与有效池、不触发同步）。

## 3. InnerAPI `/k8s_pools` 变更明细

### 3.1 模块概述改写

**改造前**：引用该池的 provider 上 `k8s_instance_pool` 为系统维护的只读镜像。

**改造后**：`k8s_pools` 是 k8s 来源实例的**唯一事实源**；provider（`instance_source=k8s_pool`）只保存 `k8s_pool_name` 引用，不保存任何实例数据；导出与 batch 在消费时实时解析。

### 3.2 PUT 全量替换实例列表

- 请求/响应结构、鉴权不变；仍返回更新后的 pool 条目（含 `last_sync_time`）。
- **执行语义变化**：只写 `k8s_pools` 单表（upsert），**不再**刷新引用 provider 的 `k8s_instance_pool` 镜像，**不再**触发 cluster 派生池 fan-out。
- 消费方（`cluster_table` 导出、batch）在下一次请求时实时读取，版本号由导出签名自动 bump。

### 3.3 DELETE pool

- 外部语义不变：**无引用**（无 `instance_source=k8s_pool && k8s_pool_name==name` 的 provider）时删除行；**有引用**时保留行并清空 `instances`（警告日志），GET 返回 200 + `instance_count=0`。
- 执行语义变化：只写 `k8s_pools` 单表；对 provider 表只读（用于判断引用）；不再 fan-out 清空 provider 镜像与派生池。
- pool 不存在仍返回 404。

### 3.4 删除"同步事务语义"章节

原 §6"同步事务语义"（PUT/DELETE 与 provider 镜像刷新、派生池 fan-out 共用同一事务）整节删除，替换为单表写语义说明。

## 4. 不变与兼容项

| 项 | 说明 |
|---|---|
| `cluster_table` 格式 | 完全不变；仅 k8s cluster 实例由实时解析产生 |
| `k8s_pools` GET 系列 | OpenAPI/InnerAPI 读接口的响应 schema 不变 |
| 手工来源 provider | 请求/响应、快照同步链路、导出行为完全不变 |
| 鉴权 | 所有端点的 feature/action 授权不变 |
| EPP / BFE / controller | 无协议变化 |

## 5. 前端影响

- provider 详情页：继续按 `k8s_pool_name` 调 `GET /open-api/v1/k8s_pools/{name}` 展示实例（#290 已落地），**无代码改动**，需回归验证。
- provider 表单：不得再读取/提交 `k8s_instance_pool`（响应已无该字段）。
- 若前端缓存了 provider 响应结构，注意字段缺失兼容性。

## 6. 文档更新记录

| 文件 | 更新内容 |
|---|---|
| `api-define/OpenAPI接口定义/providers.md` | 删除 `k8s_instance_pool` 字段（JSON 示例、字段表、校验规则）；有效池改为"实时解析"表述；PATCH 语义更新 |
| `api-define/InnerAPI接口定义/k8s-pools.md` | PUT/DELETE 改为单表写语义；删除镜像/fan-out 描述；删除"同步事务语义"章节 |
| `api-define/OpenAPI接口定义/k8s-pools.md` | 无变更（#290 只读接口不受影响） |
