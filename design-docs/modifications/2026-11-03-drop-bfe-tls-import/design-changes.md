# 证书校验改用标准库 crypto/tls：设计变更说明

## 1. 概述

### 1.1 变更背景

`ai-gateway-api` 的证书创建/更新链路有一条"证书与私钥是否匹配"的校验，实现在 `model/iprotocol/certificate.go` 的 `validateCertificate` 内，通过闭包调用 BFE 遗留包：

```go
checkCertKeyPair := func(certPEMBlock, keyPEMBlock []byte) error {
    _, err := bfe_tls.X509KeyPair(certPEMBlock, keyPEMBlock)
    return err
}
```

`bfe_tls` 是 BFE 从 Go 1.9 时代 fork 出来的 TLS 实现。BFE 数据面已完成改造（`2026-10-14-remove-bfe-tls-fork`）：删除整个包，证书选择、ALPN、ClientAuth、会话票据等全部回归标准库。api 只要 bump bfe 依赖，就会因 `bfe_tls` 缺失而编译失败。

### 1.2 变更范围

| 项 | 是否改动 |
|----|----------|
| `model/iprotocol/certificate.go` 的 key pair 校验 | ✅ 改用 `crypto/tls` |
| `bfe_config/bfe_tls_conf/*`（`server_cert_conf` / `tls_rule_conf` / `session_ticket_key_conf`） | ❌ BFE 侧保留，api 的证书导出仍使用 |
| `/certificates` 接口、错误码、操作日志 | ❌ 形状不变 |
| 存储、导出链路 | ❌ |
| `go.mod` / `go.sum` | ❌ 本变更不改（见 §4） |

### 1.3 兼容性

- **接口层**：无字段/端点变化，请求与响应形状不变；
- **错误形态**：仍为参数错误（422），message 文本可能变化（标准库错误信息同样以 `crypto/tls:` 开头，但具体措辞与 fork 版不完全一致）——错误码与结构不变，仅文本细节差异；
- **密钥范围**：标准库比 fork 更严（不支持已从 Go 移除的 P-224 等曲线），也更全（支持 Ed25519）。被收紧的部分与数据面能力一致：BFE 现在同样用标准库加载证书，控制面提前拒绝反而消除了"存得进去、下发后加载失败"的隐患；
- **可独立合入**：标准库实现与当前钉住的 bfe 版本（仍含 `bfe_tls`）同样兼容，本变更不需要等依赖 bump。

## 2. 详细改动

### 2.1 `model/iprotocol/certificate.go`

import 与调用点：

```go
// 旧
import "github.com/bfenetworks/bfe/bfe_tls"
...
_, err := bfe_tls.X509KeyPair(certPEMBlock, keyPEMBlock)

// 新
import "crypto/tls"
...
_, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
```

保留原有闭包结构与调用顺序（PEM 格式检查 → 私钥格式检查 → 匹配校验 → 过期时间解析），只替换实现来源。

### 2.2 语义对比

| 行为 | `bfe_tls.X509KeyPair`（fork） | `crypto/tls.X509KeyPair`（标准库） |
|------|------|------|
| 解析证书 PEM（可多张） | 只取 `CERTIFICATE` 块 | 相同 |
| 解析私钥 PEM | 取 `PRIVATE KEY` / `* PRIVATE KEY` | 相同 |
| 私钥类型 | RSA / ECDSA（含 P-224） | RSA / ECDSA（P-256/384/521）/ Ed25519 |
| 公钥与私钥匹配校验 | 有 | 相同（同样会解析第一张证书的 x509 公钥并比对） |
| 错误信息前缀 | `crypto/tls: ...` | `crypto/tls: ...` |

结论：调用语义等价，差异仅在密钥算法覆盖范围，且该差异方向对系统有利。

## 3. 测试

| 层 | 内容 |
|----|------|
| 单测 | 证书与私钥匹配 → 通过 |
| 单测 | 证书与私钥不匹配 → 参数错误 |
| 单测 | 证书/私钥非 PEM → 参数错误（原有分支保持） |
| 单测 | 证书缺私钥 / 私钥缺证书 → 参数错误 |
| 回归 | `go test ./model/iprotocol/...` 全绿；`go build ./...`、`go vet ./...` |
| 依赖验证 | 临时 `replace` 到已删除 `bfe_tls` 的 bfe 分支后，全仓 `go build ./...` 通过（证明 bump 前置条件解除）；验证后还原 `go.mod` |

## 4. 为什么本次不动 `go.mod`

bump 到删除了 `bfe_tls` 的 bfe 版本还缺两个条件：

1. 上游需发布含 `AIConf.EnsureReasoningContent` 的版本（另一变更的依赖，见 `2026-11-03-cluster-reasoning-content-protocols/design-changes.md` §6）；
2. 新版本引入的传递依赖（`bfe_expr` → `expr-lang/expr`）需要在 bump 时由 `go mod tidy` 写入 `go.mod` / `go.sum`。

把本变更与 bump 拆开的好处：`bfe_tls` 依赖现在就是债务，早一天解除早一天安全；bump 则等上游节奏，互不阻塞。

## 5. 实施阶段

| 阶段 | 内容 | 状态 |
|------|------|------|
| 1 | 代码替换（§2.1） | ✅ 已完成 |
| 2 | 单测与回归（§3） | ✅ 已完成（见 §6） |
| 3 | 依赖验证：replace 到 bfe 分支可编译 | ✅ 已完成 |
| 4 | 文档沉淀评估（Step 6） | ✅ 已在 §7 记录结论 |

## 6. 实现结果

- **代码**：`model/iprotocol/certificate.go` 去掉 `bfe_tls` import，改用 `crypto/tls.X509KeyPair`；全仓 grep 确认 `bfe_tls` 仅此一处（`bfe_tls_conf` 是配置结构体包，BFE 侧仍保留，不在本次范围）；
- **测试**：`go test ./model/iprotocol/...` 通过；既有证书用例（匹配/不匹配/格式非法）覆盖该分支，无需新增用例；
- **验证**：当前钉住版本下 `go build ./...`（除另一变更的字段依赖外）与 `go vet` 通过；`replace` 到 bfe 分支（已删 `bfe_tls`）后全仓 `go build ./...`、`go vet ./...`、`go test ./model/... ./lib/... ./endpoints/...`、`test/integration` 子模块 `go build` 全部通过；
- **`go.mod` 的当前状态**：为了在同一工作区同时验证"TLS 适配 + `EnsureReasoningContent` 字段"这一最终形态，`go.mod` 现在**保留了一条带 `TODO(dep-bump)` 注释的临时 `replace => ../bfe`**，`go mod tidy` 的连带结果也一并保留（`+ github.com/expr-lang/expr`、`- github.com/tjfoc/gmsm`，后者原为 `bfe_tls` 的 SM2 依赖）。上游发布含该字段的版本后需删除 `replace` 并改为正常版本 bump——**该行不得随功能分支合入主线**；
- **未做**：正常的版本 bump（等待上游发布，见 §4）。

## 7. 变更沉淀评估（Step 6）

不新增 `sys-design/details/` 文档。理由：本变更只是把一个校验函数的实现来源从 fork 换成标准库，未引入新机制、新数据流或新状态；相关系统设计（`/certificates` 域、证书导出）不受影响。记录在 `modifications/` 即可，供依赖升级时追溯。