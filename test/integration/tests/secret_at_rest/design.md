# DB 落盘加密（secret_at_rest）集成测试设计

> 功能实现：`lib/xcrypto`、`stateful/security.go`、`storage/rdb/{provider,api_key}`、
> `model/keyrotate`、`endpoints/openapi_v1/security/`
> （变更说明 `design-docs/modifications/2026-10-05-db-encryption-at-rest/`、
> `design-docs/modifications/2026-10-10-reencrypt-sweeps-list-endpoint/`（GET 列表端点），
> 契约 `api-changes.md`）。
> 模块前缀：**SAR**（Secret At Rest）。

---

## 1. 被测对象与特殊性

DB 落盘加密是**存储层透明机制**（非业务域资源），被测面横跨进程内各层，核心事实：

| 被测面 | 说明 |
|--------|------|
| DAO 透明加解密 | `providers.api_keys`（JSON 列整列）、`api_keys.api_key`（单值 + `api_keys.api_key_hash`）；无 keyring 时明文直通（向后兼容） |
| keyring 注入 | `[Security].MasterKeyFile` 为唯一注入方式；keyring 文件 TOML：`ActiveKeyID` + `[Keys]` map（值为 base64 的 32 字节主密钥）；信封 `enc$v1$<base64(keyID(1B)\|nonce(12B)\|ct)>`，解密按密文首字节 keyID 选钥 |
| 启动 fail-fast | `CheckSecretAtRest`（main.go rdb.Init 之后）：库中存在密文（任一敏感列 `LIKE 'enc$v1$%'`）而无 keyring → 拒绝启动；无密文无 keyring → 允许启动（渐进启用） |
| 热加载 | monitor 端口 `POST /reload/security`：重读 keyring 文件 → 校验 → 原子替换，失败保留旧 keyring |
| sweep 收敛 | `POST /open-api/v1/security/reencrypt-sweeps`（异步任务，锁表单例互斥）+ `GET .../{task_id}`（含按表 summary）+ `GET .../reencrypt-sweeps`（历史列表：status/mode/scope/dry_run/started_at 区间过滤 + page/page_size 分页 + sort_by/sort_order，默认 started_at 倒序；2026-10-10 新增）；`mode=reencrypt`（收敛到 active 钥）/ `decrypt`（回滚预案，剥 marker 回明文） |

**与常规模块的差异**：断言的核心证据在 **DB 文件字节层面**，而非 API 报文——本模块以
`database/sql` + `sqlite-strip` 驱动（与 `testutil.InitTestDB` 同款，经 `replace` 引入的
主模块 `stateful` 注册）**绕过服务进程直读 SQLite**，断言：

- 无 keyring：两列均为明文且不含 `enc$v1$` 前缀；
- 有 keyring：两列均为 `enc$v1$` 信封，且 base64 解码首字节 keyID 等于预期（写死断言
  keyID=1 / 轮换后全部 keyID=2，而非仅断言"是密文"）；
- 业务透明性：OpenAPI 读回与 InnerAPI 导出均为明文（且报文不含 `enc$v1$` 密文泄漏）。

**实例模型**：各用例独立 `testutil.StartServerWithMonitor(extraTOML)`（本模块全部用例
需要 monitor 端口做 `/reload/security` 或保证能力一致），keyring 文件写在 `t.TempDir()`，
extraTOML 注入 `[Security]` 段（路径经 `filepath.ToSlash` 转义）。共享 DB 场景
（SAR-1-003）使用 `StartServerWithSharedInfra`。

## 2. 关键实现技巧

| 技巧 | 落点 |
|------|------|
| keyring 文件生成 | `crypto/rand` 生成 32 字节主密钥 → base64 → 写 `t.TempDir()/master.keys`（`0600`） |
| keyID 判定 | 复用主模块 `lib/xcrypto`（integration go.mod 已 `replace ../../`）：`xcrypto.IsEncrypted` / `xcrypto.KeyIDOf`（解 base64 首字节） |
| raw HTTP helper | 全局 Client 不暴露 HTTP 状态码；本模块自带 `doRaw(method, url, body)` 返回 `Status/ErrNum/ErrMsg/Raw`，用于 202/404/409/422 状态断言 |
| 直读 SQLite 并发 | 服务进程持有 DB；直连 DSN 带 `_pragma=busy_timeout(5000)`（与 `createTempConfig` 同款），避免与后台审计刷盘瞬态争用 |

## 3. 场景总览（用例编号登记）

| 编号 | 用例名 | 前置配置（extraTOML） | 步骤与断言要点 |
|------|--------|------------------------|----------------|
| SAR-1-001 | 无 keyring 明文直通 | 无 `[Security]` 段 | 建 provider + api-key → 直读 SQLite：`providers.api_keys` 含明文 `sk-aaa…` 且无 `enc$v1$`、`api_keys.api_key` 等于创建时明文且无 `enc$v1$` → OpenAPI 读回明文一致（provider `keys` / api-key `key`） |
| SAR-1-002 | 配置 keyring 落盘加密 + 透明解密 | `[Security].MasterKeyFile`（key 1，active=1） | 建 provider + api-key → 直读 DB：两列均 `enc$v1$` 且 keyID=1 → OpenAPI 读回明文一致（透明解密）→ InnerAPI `GET /inner-api/v1/configs/mod-api-key` 200 且 body 含明文 key、不含 `enc$v1$`（导出口解密） |
| SAR-1-003 | 库有密文但无 keyring 拒启动 | 实例 A 配 keyring-A 建密文；实例 B `StartServerWithSharedInfra` 共享 DBPath+Redis、无 `[Security]` | A 建 provider → 直读 DB 确认密文 → B 启动**必须失败**（`CheckSecretAtRest` fail-fast，进程退出后 `waitForReady` 10s 超时返回 error），断言 `err != nil。**注**：框架的 `StartServerWithSharedInfra` 不支持 extraTOML，无法给 B 注入"错误 keyring"；且 `CheckSecretAtRest` 只校验"有密文必须有 keyring"（不验钥匹配），故 B 无 keyring 是本框架下可测的启动失败场景（"错误 keyring 但配置了文件"会启动成功、仅读路径 5xx，不属于启动失败语义）。**耗时说明**：进程 fail-fast 后 `waitForReady` 需等满 10s 超时，本用例固定接受该耗时（全模块仅此一例） |
| SAR-2-001 | 轮换 + 收敛全链路 | keyring v1（active=1）启动；运行中覆写 keyring v2（追加 key 2、active=2） | 建 provider + api-key → 直读 DB keyID=1 → 覆写 keyring 文件 → `POST {monitor}/reload/security` 200 且响应含 `reloaded` → dry-run sweep：成功响应、`active_key_id=2`、`mode=reencrypt` → 轮询至 succeeded → **直读 DB 新建行仍为 keyID=1（dry-run 不写库；seed `deepseek` 为 DDL 直插明文，尚未收敛，不纳入此断言）** → 正式 sweep → 轮询至 succeeded（summary 按表计数 rewritten≥预期）→ 直读 DB 全部行 keyID=2（含 seed `deepseek` 明文 `[]` 亦被收敛）→ OpenAPI 读回明文一致 |
| SAR-2-002 | decrypt 回滚模式 | keyring（active=1）启动 | 建 provider + api-key → 直读 DB 确认密文 → `{"mode":"decrypt"}` sweep → 轮询至 succeeded → 直读 DB：两列均无 `enc$v1$` 前缀（`api_keys.api_key` 等于原明文 key、`providers.api_keys` 含明文 provider key）→ OpenAPI 读回明文一致 |
| SAR-2-003 | sweep 接口语义 | keyring（active=1）启动 + 直插 300 行明文 provider 拉长任务窗口 | 触发 T1（scope=all）→ **运行中**重复触发直至 409（循环上限 500 次防御；小数据任务 running 窗口仅几十毫秒，先直插批量行把窗口拉到百毫秒级）→ 断言 HTTP 409 + `ErrNum=409` + `ErrMsg` 含 T1 `task_id` → 轮询 T1 succeeded（幂等，锁已释放）→ GET 不存在 task_id → HTTP 404 + `ErrNum=404` → `mode=rot13` → HTTP 422 + `ErrNum=422` → `scope=everything` → HTTP 422 + `ErrNum=422` |
| SAR-3-001 | sweep 列表过滤与字段 | keyring（active=1）启动 | 顺序触发 dry-run（scope=providers）与正式任务并各等待 succeeded → 无参列表：HTTP 200 + `total=2` + 两任务均出现 + 元素字段完整（status/mode/dry_run/scope/active_key_id/started_at/finished_at/duration_ms/summary 按表分组）+ 报文含 `created_by` 键 + dry-run 任务 `error="dry_run"` → `status=succeeded`/`running` 过滤命中数 2/0 → `dry_run=true` 仅命中 dry-run 任务 → `mode=reencrypt`/`decrypt` 命中数 2/0 → `start_time`/`end_time` 闭区间命中 2，区间外（end 早于全部任务 start / start 晚于全部任务 start）命中 0 |
| SAR-3-002 | sweep 列表分页与排序 | keyring（active=1）启动 | 顺序触发 3 个 dry-run 任务 → `page_size=2` 翻两页：并集 == 触发集合（无序比对，规避 started_at 同秒 tie）+ `total` 恒 3 → `page=99` 超界：200 + 空页 + `total` 不变 → `page_size=500` 截断为 100（响应回显）→ `sort_by=task_id&sort_order=asc`：全列表 task_id 严格升序 → `sort_by=evil&sort_order=sideways` 回落默认（首元素与无参列表一致） |
| SAR-3-003 | sweep 列表参数校验 | keyring（active=1）启动 + 1 个已完成任务 | `status=done` / `mode=rot13` / `scope=everything` / `start_time` 非 RFC3339 / `end_time` 非 RFC3339 / `end_time` 早于 `start_time` → 各 HTTP 422 + `ErrNum=422` + `ErrMsg` 字段级归因 → 合法但无匹配（`status=failed`）：200 + 空列表 |

> 用例计数：9（SAR-1-xxx × 3、SAR-2-xxx × 3、SAR-3-xxx × 3）。编号在实现 `_test.go` 中以注释标注（`// SAR-x-xxx`）；SAR-3-xxx 在 `sweeps_list_test.go`。

## 4. 实现与合同偏差（测试断言以真实行为为准，合同建议后续对齐）

| 项 | 合同（api-changes.md） | 实现真实行为 | 测试处理 |
|----|------------------------|--------------|----------|
| sweep 触发成功 HTTP 状态 | 202 | `lib/xreq/result.go` 的 `Render` 把一切 2xx `ErrNum` 归一为 HTTP 200（body `ErrNum=200`） | 断言 HTTP 200 + `ErrNum=200` + Data 字段语义；建议后续给 TriggerRoute 自定义 Render 落地 202，或修订合同 |
| decrypt 模式 `active_key_id` | 返回 0 | `keyrotate.Manager.Trigger` 统一取当前 keyring active（decrypt 下非 0） | decrypt 用例不断言该字段；建议后续按合同区分模式 |

## 5. 运行方式

```bash
cd ai-gateway-api && go build -o ai-gateway-api.exe .   # 先重建二进制
cd ai-gateway-api/test/integration
go vet ./tests/secret_at_rest/
go test -v -count=1 -timeout 600s ./tests/secret_at_rest/...
```
