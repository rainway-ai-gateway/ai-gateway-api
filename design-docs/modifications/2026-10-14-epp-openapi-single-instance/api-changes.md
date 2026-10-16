# OpenAPI 单实例配置 + 视图去除 standby API 变更

## 1. `PATCH /epp-pool`：组规模校验

**Before**：每组 1 个（仅主）或 2 个（主+备）实例；拒绝空组与 3 个及以上实例的组。

**After**：每组**恰好 1 个**实例；拒绝空组与 2 个及以上实例的组。

422 错误文案：

- Before：`epp pool group "g1" requires 1 or 2 instances, got 3`
- After：`epp pool group "g1" requires exactly 1 instance, got 2`

请求/响应结构（`groups[].instances[]` 的 id/host/port）不变。

## 2. `GET /epp-assignments` 响应

**Before**：

```json
{
  "clusters": [
    {
      "cluster": "cluster-a",
      "group": "g1",
      "primary":   { "id": "epp-a", "host": "10.0.0.1", "port": 9002 },
      "standby":   { "id": "epp-b", "host": "10.0.0.2", "port": 9002 },
      "degraded": false
    }
  ],
  "unassigned_clusters": [],
  "idle_groups": ["g2"]
}
```

**After**（`standby` key 物理删除）：

```json
{
  "clusters": [
    {
      "cluster": "cluster-a",
      "group": "g1",
      "primary":   { "id": "epp-a", "host": "10.0.0.1", "port": 9002 },
      "degraded": false
    }
  ],
  "unassigned_clusters": [],
  "idle_groups": ["g2"]
}
```

查询参数（`cluster` 过滤）、错误语义（404/422）不变。

## 3. `PUT /epp-assignments/{cluster}` 响应

元素结构由 `{cluster, group, primary, standby}` 变为 `{cluster, group, primary}`，其余（请求体、错误语义、version bump 行为）不变。

## 4. 明确不变

| 出口 | 说明 |
|------|------|
| InnerAPI `GET /configs/epp_data/config` | `Config.assignment[cluster] = {primary, standby}` 不变；单实例组下 `standby` 为 `null`（既有语义），存量双实例组继续输出备实例 id |
| server_data_conf `GslbBasic.EPPAddr` | 有序 `[主, 备]` 语义不变；单实例组下仅 `[主]`（既有语义） |
| 分配器 / 悬空修复 / reconciler | 行为不变 |
