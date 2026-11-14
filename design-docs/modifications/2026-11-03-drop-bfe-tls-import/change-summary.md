# 证书校验改用标准库 crypto/tls（去掉 bfe_tls 依赖）——变更摘要

## 1. 背景

`ai-gateway-api` 在证书创建/更新时校验"证书与私钥是否匹配"，调用的是 BFE 遗留的 `bfe_tls.X509KeyPair`（`model/iprotocol/certificate.go:174`），仅为拿它的 error。

`bfe` 数据面已完成 TLS 改造（见 `bfe/docs/zh_cn/modifications/2026-10-14-remove-bfe-tls-fork/design-changes.md`）：删除整个 `bfe_tls` 包，TLS 能力回归标准库 `crypto/tls`。因此控制面只要 bump `bfe` 依赖就会编译失败：

```
model\iprotocol\certificate.go:24:2: no required module provides package
    github.com/bfenetworks/bfe/bfe_tls
```

这是"集群 `AIConf.EnsureReasoningContent` 控制面下发"变更（`2026-11-03-cluster-reasoning-content-protocols`）依赖 bump 的前置阻塞项，本变更负责解除它。

## 2. 目标

1. 控制面证书校验改用标准库 `crypto/tls.X509KeyPair`，去掉对 `bfe_tls` 的唯一依赖；
2. 校验语义与数据面实际能力对齐：BFE 现在用标准库加载证书，控制面用同一套实现校验，避免"控制面放行、数据面加载失败"；
3. 不改变任何 API 行为与错误码形态；不引入新依赖。

## 3. 范围

- **涉及面**：`model/iprotocol/certificate.go` 的证书-私钥匹配校验，及对应测试；
- **不涉及面**：`bfe_config/bfe_tls_conf/*`（配置结构体，BFE 侧仍保留，api 继续使用）、证书接口与导出链路、数据库；
- **依赖面**：本变更**不**修改 `go.mod`——标准库实现与当前钉住的 bfe 版本同样兼容，可独立合入；完成本变更后，`bfe` 依赖才可以 bump 到已删除 `bfe_tls` 的版本。

## 4. 关键决策

| 决策 | 说明 |
|------|------|
| 用 `crypto/tls` 而非自研校验 | `bfe_tls.X509KeyPair` 本身就 fork 自该函数（错误信息同样以 `crypto/tls:` 开头）；自研等于复制一份标准库，无收益 |
| 只做匹配校验，不引入会话能力 | 调用方丢弃返回值（`_, err :=`），仅需 error；不搬 `Certificate` 结构 |
| 接受标准库更严的密钥范围 | fork 为 Go 1.9 时代实现，支持 P-224 等标准库已移除的曲线；标准库对这类证书报错。由于**数据面也已是标准库**，控制面提前拒绝才是正确行为（否则会放行一份 BFE 加载不了的证书） |
| 不在本次 bump 依赖 | 解除阻塞 ≠ 完成 bump：bump 还要求上游 bfe 发布含 `EnsureReasoningContent` 的版本，属另一变更 |

## 5. 关联文档

- 详细设计：`design-changes.md`
- 阻塞方（本变更的消费方）：`2026-11-03-cluster-reasoning-content-protocols/`（其 `design-changes.md` §6 记录了两条路线，本变更即路线 A 的前置改造）
- BFE 侧 TLS 改造：`bfe/docs/zh_cn/modifications/2026-10-14-remove-bfe-tls-fork/design-changes.md`

## 6. 实施阶段

| 阶段 | 内容 | 状态 |
|------|------|------|
| 1 | 证书校验改用 `crypto/tls.X509KeyPair` | ✅ 已完成 |
| 2 | 单测（匹配/不匹配/格式非法） | ✅ 已完成（既有用例覆盖） |
| 3 | 验证：`replace` 到 bfe 分支后全仓可编译（证明 bump 前置条件已解除） | ✅ 已完成 |
| 4 | 正常版本 bump（等待上游发布含 `EnsureReasoningContent` 的版本） | ⏳ 待外部条件 |

## 7. 实现结果

- `model/iprotocol/certificate.go` 去掉 `bfe_tls` import，改用标准库；全仓仅此一处引用；
- `replace` 到 bfe 分支后：`go build ./...`、`go vet ./...`、`go test ./model/... ./lib/... ./endpoints/...`、`test/integration` 子模块 `go build` 全部通过；
- `go.mod` 当前保留带 `TODO(dep-bump)` 注释的临时 `replace => ../bfe`（配套 `+ expr-lang/expr`、`- tjfoc/gmsm`），**不得随功能分支合入主线**；上游发版后删除并改为正常 bump。