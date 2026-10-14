# OpenAPI 单实例配置 + 视图去除 standby 设计变更

## 1. 分层策略（核心设计）

本次变更刻意**只动写入校验与 OpenAPI 出口**，内部链路原样保留：

```
PATCH /epp-pool ──validateGroups──> epp_instances（DB）
                                        │
        ┌───────────────────────────────┼───────────────────────────────┐
        │                               │                               │
  GET /epp-assignments           EppDataGenerator                 GetAssignmentEndpoints
  （OpenAPI 视图）                （InnerAPI epp_data）           （EPPAddr 导出）
        │                               │                               │
  epp_assignments/endpoints/      model/epp_pool/epp_data.go     model/icluster_conf
  data.go 删 Standby 字段         ★ 不动：standby 照常展开        ★ 不动：[主,备] 照常导出
  （standby key 消失）            （新数据下恒 null）              （新数据下恒 [主]）
```

- **端点层是唯一的对外契约变化点**：`ClusterAssignmentData` / `ClusterAssignmentOverrideData` 删除 `Standby` 字段后，OpenAPI 响应不再携带 `standby` key。
- **模型层 `ClusterAssignmentView` 保留 `Standby`**：`epp_data` 生成器（`EppDataGenerator`）与 `EPPAddr` 导出继续消费它；standby 读时展开逻辑（`expandPrimaryAndStandby`）继续服务存量双实例组数据。

## 2. 改动明细

### 2.1 `model/epp_pool/epp_pool.go`

```go
// before
func (m *EppPoolManager) validateGroupSize(group *InstanceGroup) error {
	if n := len(group.Instances); n < 1 || n > 2 {
		return xerror.WrapParamErrorWithMsg("epp pool group %q requires 1 or 2 instances, got %d", group.Name, n)
	}
	return nil
}

// after
func (m *EppPoolManager) validateGroupSize(group *InstanceGroup) error {
	if n := len(group.Instances); n != 1 {
		return xerror.WrapParamErrorWithMsg("epp pool group %q requires exactly 1 instance, got %d", group.Name, n)
	}
	return nil
}
```

`assignment.go` `candidateGroups` 注释中 "PATCH 校验保证每组 1~2 实例" → "PATCH 校验保证每组恰 1 实例"。

### 2.2 `endpoints/openapi_v1/epp_assignments/data.go`

- `ClusterAssignmentData` 删除 `Standby *InstanceData`；
- `ClusterAssignmentOverrideData` 删除 `Standby *InstanceData`；
- 两个构造函数删除 `Standby: newInstanceData(view.Standby)`。

### 2.3 明确不动的文件

`model/epp_pool/assignment.go`（除上述注释）、`model/epp_pool/epp_data.go`、`model/icluster_conf/*`、`endpoints/innerapi_v1/epp_data/*`、存储层、`model/epp_pool/*_test.go` 中验证内部视图的用例（`TestBuildAssignmentsView` 的 standby 断言、export 相关断言等继续覆盖保留行为）。

## 3. 兼容性论证

1. 单实例组下 `expandPrimaryAndStandby` 天然返回 `standby=nil`，`epp_data`/`EPPAddr` 导出对该形态的输出（`standby: null`、单元素 `EPPAddr`）在 `2026-09-14-epp-pool-unify-group-size` 已定义并被 BFE/EPP 消费，零配合成本。
2. 存量双实例组：读取/导出/悬空修复路径一行未改，行为完全不变。
3. OpenAPI 唯一 breaking：`standby` key 物理消失（非恒 null），忽略未知 key 的调用方无感。

## 4. 测试策略

- **单测**：`epp_pool_manager_test.go`（FullReplace/Validation/新增 two-instance rejected）、`epp_pool/endpoints_test.go`（PatchAction body、GroupSize 新增 2 实例拒绝）、`epp_assignments/endpoints_test.go`（删 standby 断言 + 新增"响应无 standby key"JSON 断言）。模型层单测不动。
- **集成测试改写**：`epp_pool/update`（EP-2-001/013/014/015）、`epp_assignments/get`、`epp_assignments/put`、`schema/openapi`（epp_assignments.go schema + openapi_schema_test.go）。
- **原 InnerAPI 用例同步（2026-10-14 完成）**：以下用例原验证 `[主,备]` 导出、夹具需双实例组，已改写为单实例组形态——`EPPAddr` 断言改单元素，OpenAPI 视图断言改无 `standby` key，`epp_data` 导出保留 `standby: null` 断言（InnerAPI 契约未变）：

| 文件 | 用例 |
|------|------|
| `tests/innerapi/epp_data/epp_data_test.go` | IN-EPP-002/003/005 |
| `tests/schema/innerapi/innerapi_schema_test.go` | `testServerDataConfEppSchema` |
