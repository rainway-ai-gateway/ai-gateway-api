<!--
This changelog should always be read on `master` branch. Its contents on other branches
does not necessarily reflect the changes.
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [0.1.0] - 2026-11-14

### Added
- `llm_config.ensure_reasoning_content` cluster config: the model access protocols for which the gateway fills an empty `reasoning_content` on assistant messages after the last user message (pairs with `AIConf.EnsureReasoningContent` in BFE v1.9.0); also removes the `bfe_tls` dependency in favor of stdlib `crypto/tls` and closes the provider protocol-narrowing gap.
- Key-affinity penalty duration configuration: `llm_config.key_affinity` gains `penalty_ttl_429` / `penalty_ttl_auth` (seconds, issue #1448), paired with the configurable AI upstream key penalty in BFE v1.9.0.
- Missing-price action and signals: `llm_config.price_miss_action` (`fallback_chat` default / `bill_zero` / `reject`, validated against the same enum as BFE `AIConfCheck`, exported as `AIConf.PriceMissAction`, issue #1479); report detail gains the `ai_price_miss` / `ai_price_warn` columns with an exact `price_warn` filter (whitelist-validated).
- Unpaginated full-list responses: `/entities` and `/entity-types` return the complete list when pagination parameters are absent.

### Changed
- Upgrade data-plane dependency `github.com/bfenetworks/bfe` to the formal v1.9.0 release; the temporary `replace` to a fork pseudo-version of the v1.9.0-dev branch is removed (the fork commit is contained in the release).
- EPP default timeout parameters made self-consistent: `queue_ttl` 9s / `timeout.call` 11s.

### Fixed
- Report overview/timeseries TTFT/TPOT converge to the streaming caliber (only `ai_stream=1` rows), identical to the Doris / StarRocks / ClickHouse aggregate rebuilds, so mixed-traffic averages match the observability backends (issue #315).
- Quota: the used-quota derivation no longer clamps at zero — overage beyond the plan is reported as-is (issue #1476).
- List endpoints backfill default pagination (page 1, size 20) when pagination parameters are absent, with `total` computed against the filter (issue #303).
- HTTP layer: 405 responses now carry an `Allow` header listing the methods actually supported on the path (issue #297); CORS preflight and the Req-ID header now also cover router-level 404/405 responses (issue #298).
- Rule-set whole-replace (PUT on rule collections) is serialized per resource — concurrent conflicting writes return 409 instead of racing (issue #295).
- Validation hardening: RMB quota decimal places counted by decimal representation, not binary (issue #293); description length limits unified to UTF-8 bytes across domains (issue #310); name-charset validation on refer-by-name endpoints returns 422 for illegal names (issue #308); provider / model / base_model gain 1-255 length validation (issue #309); `instance_pool[].addr` gains hostname validation (issue #321); cluster and entity-type GET/DELETE by-name endpoints validate the name from the URI (issues #311/#320); entity-type `PATCH level` gains 1-5 validation with hierarchy-consistency protection (issue #313).
- Conflict semantics: deleting an Entity-Type still referenced by Entities returns 409 (issue #319); deleting an Entity still mounted by API keys returns 409 (issue #316).
- Provider `PATCH` partial updates validate the merged object instead of the raw patch body (issue #322).
- Same-second concurrent config exports absorb the `uk_name_version` unique-key conflict instead of failing the whole request with 500 (issue #318).
- RouteRules rejects `null` elements in `rules` / `targets` / `fallbacks` instead of returning 500 (issue #327 follow-up).

## [0.0.11] - 2026-10-24

### Added
- Batch and async task management (phase 1): OpenAPI batch/file lifecycle aligned with the data plane's `mod_ai_batch` passthrough — terminal-field persistence, backstop settlement parsing on result-file download, and documented `batch_files` metadata.
- Report query backends: ClickHouse and StarRocks (`[Report].Backend` third/fourth form, zero go.mod impact), Doris phase-2 aligned with the cache/mirror/intent dimensions, and report dimensions for the new `ai-cache` / `traffic-mirror` / `intent` access-log fields.
- Security suite: user passwords stored as bcrypt hashes (SKIP backdoor removed, legacy plaintext lazily migrated on login); DB at-rest encryption for key material (envelope encryption + keyring + hot reload + convergence sweeps); field-level `enc$v1$` encryption for exported config files (control-plane landing of the data plane's `[Security]` keyring, BFE v1.8.9); management-plane IP allowlist (server config + middleware + hot reload).
- Reencrypt-sweep history endpoint (`GET` list with filter/pagination/`created_by`) for auditing past sweep runs.
- `ai_cache_rules` collection resource with `mod_ai_cache` config export, including the semantic-cache control plane (per-rule `enableSemantic` switch, global semantic settings, top-level `Semantic` export block).
- `traffic_mirror_rules` collection resource with `mod_traffic_mirror` config export.
- `intent_config` singleton resource with `mod-ai-intent` config export.
- AI context compression control plane (rule set + global settings + top-level `Defaults` export block).
- K8s provider instance pools: `instance_source` + `k8s_pools` maintenance for Kubernetes-discovered backends.
- `clusters.llm_config.normalize_upstream_error` config for upstream error normalization (pairs with `AIConf.NormalizeUpstreamError` in BFE v1.8.9).
- BFE condition primitive `time_weekday_in` support (aligned with the upgraded data-plane dependency).
- Auth: system-administrator (read-only) role added; `is_admin` deprecated in favor of the unified `role`; API key responses masked.
- Cluster EPP runtime parameters (`epp_runtime`) in cluster conf; new `database/` directory (`db_ddl_upgrade_to_0.0.11.sql` + report SQLite DDL).
- SQLite single-machine local deployment; release package contents completed.

### Changed
- **Breaking** `PATCH /epp-pool`: tighten group-size validation to exactly 1 instance per group (backup/standby pairs are no longer accepted); the `instances` field keeps its array form reserved for future multi-instance expansion.
- **Breaking** `/epp-assignments`: assignment granularity is now the EPP group, not a single instance. The view is reduced to `{cluster, group}` (`primary`/`standby`/`instances`/`degraded` removed; instance details are derived from `/epp-pool` by group name); the PUT body is now `{"group_name"}` only. Clusters without a valid assignment (never assigned or dangling) are enumerated in `unassigned_clusters` and repaired by the periodic reconciler.
- **Breaking** InnerAPI `GET /configs/epp_data/config`: `assignment[cluster]` entry changed from `{primary, standby}` to `{group}`. ai-gateway-epp ships in the same release with group-based identity matching (`-group-name`); no cross-version compatibility — mixed versions fail loudly via `ai_epp_assignment_no_match`; see `design-docs/modifications/2026-10-14-epp-single-instance/`.
- **Breaking** schema: drop the `epp_assignments.primary_instance_id` column (appended to `database/db_ddl_upgrade_to_0.0.11.sql` §4, with rollback notes; converge existing pools to single-instance groups before upgrading).
- `server_data_conf` `GslbBasic.EPPAddr` is now always a single-element list (the assigned group's instance addresses); no BFE change required.
- Integration tests (`test/integration`) now default to a **MySQL** backend for `StartServer`/`StartServerWithMonitor`/`StartServerWithSharedInfra` (unique `ai_gateway_it_*` database per start, dropped on shutdown): SQLite's `TEXT` columns never validate JSON, which hid MySQL-only failures like the `providers.api_keys` `Error 3140` regression (issue #264) from the entire suite. Backend selection: `AIAPI_FORCE_SQLITE=1` (opt-out) → `AIAPI_MYSQL_DSN` (admin DSN without database name) → probe local `root@tcp(127.0.0.1:3306)/` → fall back to SQLite. `testutil.OpenDB(t, sm)` is the backend-agnostic way to read the database under test. See `design-docs/modifications/2026-10-21-integration-tests-mysql-default/`.
- **Breaking** `GET/PUT /open-api/v1/ai-cache-rules`: rule entries no longer carry `created_at`/`updated_at`. A full-replace collection recreates every row on each PUT (ids renumbered), so per-rule timestamps only ever equalled the last PUT time; they are now server-maintained columns only, and audit time is available via `GET /operation-logs`. Submitted `created_at`/`updated_at` keys are ignored as unknown fields, so existing clients keep working. Aligns the resource with `/ai-context-rules` and `/global-route-rules`; see `design-docs/modifications/2026-10-20-issue-258-ai-cache-rules-drop-readonly-timestamps/`.
- **Breaking** EPP exposed config refactored to load profile + admission watermark + bad-backend eviction; `scheduling_profile` and `cache_affinity` removed from the EPP config surface.
- Upgrade data-plane dependency `github.com/bfenetworks/bfe` to v1.8.9 (formal release); the `replace` directive pinning a fork pseudo-version is removed, so the released module is consumed directly.

### Removed
- **Breaking** `providers.k8s_instance_pool`: the system-maintained read-only mirror column is dropped entirely (`db_ddl.sql` / `db_ddl_sqlite.sql` / `database/db_ddl_upgrade_to_0.0.11.sql` §1.4 — the column never shipped with v0.0.11; preview databases that added it must run `ALTER TABLE providers DROP COLUMN k8s_instance_pool` manually). Provider responses no longer carry the field, and request bodies that still send it are silently ignored as an unknown field. K8s-mode instances are now resolved at read time from `k8s_pools` (cluster_table export and batch egress), with no mirror refresh or fan-out; see `design-docs/modifications/2026-10-27-k8s-pool-realtime-resolution/`.

### Fixed
- `GET /open-api/v1/batches/{batch_id}` now returns the task object itself at the `Data` top level with the associated files in `batch_files` (shape per `batches.md` §3.2), instead of the `{task, files}` wrapper; file entries follow the documented metadata shape and no longer expose internal fields (`id`/`direction`/`key_name`/…). This fixes the batch task detail page rendering empty panels in the dashboard (issue #249).
- `/entity-types` `description` now rejects values longer than 255 bytes on both create and partial update (`422`); previously the limit was mis-set to 256, so a 256-byte description was accepted and stored. Matches `entity-types.md` §1/§2.1 (0-255 bytes) — issue #244, unblocking SC1203-TC011/TC015/TC017.
- `/clusters` `description` is now validated on create and partial update: length must be ≤ 256 bytes and control characters are rejected (`422`). Previously the field had no validation at all, so any length (e.g. 257 bytes) and control characters were accepted and stored. Matches `clusters.md` §1/§2.1 (0-256 bytes, no control characters) — SC2101-TC024.
- `description` length limits for `/entity-types`, `/entities` and `/clusters` are documented and counted in UTF-8 bytes, and the limit constants are now named per domain (`MaxEntityTypeDescriptionLength` / `MaxEntityDescriptionLength` / `MaxClusterDescriptionLength`) instead of the ambiguous shared `MaxDescriptionLength`; see `design-docs/modifications/2026-10-17-issue-244-description-length-validation/`.
- `POST /open-api/v1/certificates` now validates `description`: length must be 2-256 bytes and control characters are rejected (`422`); a rejected create leaves no certificate behind. Previously only `required,min=2` was enforced, so descriptions longer than 256 bytes and control characters were accepted and stored. Matches `certificates.md` §1/§2.1 (required, 2-256 bytes, no control characters); see `design-docs/modifications/2026-10-17-cert-description-validation/`.
- `GET /open-api/v1/operation-logs` filtered first-page queries (`operator_name` / `status` / `resource_name` / `resource_id`) no longer take 3.6–20 s on existing databases upgraded via the incremental script. The filter-column composite indexes from issue #275 reached only new installs (`db_ddl.sql`); `database/db_ddl_upgrade_to_0.0.11.sql` gains a §5 delivering the missing `ALTER TABLE` statements to existing deployments, plus a new `(resource_id, created_at)` index covering `resource_id`-only filters (the existing `idx_resource` leads with `resource_type`), and `pagination.total` is now served from a short-TTL Redis cache keyed by filter with generation invalidation on writes, so paging through one filtered result set no longer repeats the full count on every page (issue #280).
- `GET /open-api/v1/operation-logs` default ordering is now deterministic for logs created within the same second: `created_at DESC, id DESC` (id tiebreaker). Operation logs flush asynchronously in batches, so several entries often share the same second-level timestamp; with `created_at` alone the within-second order was storage-dependent (MySQL vs SQLite returned different orders), which could surface the wrong entry to callers that page from the newest — exposed by the integration suite moving to MySQL as its default backend.
- `POST /open-api/v1/providers` and `PATCH /open-api/v1/providers/{provider_name}` no longer fail with `500 Database Exception: Error 3140 ... for column 'providers.api_keys'` when DB at-rest encryption is enabled. `providers.api_keys` held a whole-column `enc$v1$` envelope while the column was declared `JSON`, which MySQL rejects on write (strict mode); it is now `MEDIUMTEXT` (`db_ddl.sql` for new installs, `database/db_ddl_upgrade_to_0.0.11.sql` §1.6 for existing ones — JSON→text conversion requires `ALGORITHM=COPY, LOCK=SHARED`). The same constraint also broke `scope=providers` reencrypt sweeps, so existing plaintext rows could not converge on MySQL. A new startup check (`stateful.CheckEncryptedColumnTypes`, active only with a keyring) refuses to start with an actionable message when the column is still `JSON`, and `fromDAO` now fails loudly on a corrupt key list instead of silently reading it as "no keys"; see `design-docs/modifications/2026-10-21-issue-264-provider-api-keys-column-type/`.
- `PUT /open-api/v1/ai-cache-rules` now rejects duplicate `cond` values within the submitted collection (422, collection unchanged). Previously such a collection was accepted and exported verbatim, so BFE `mod_ai_cache` rejected the whole `ai_cache.data` (`can't have same cond`) while conf-agent retried the same unloadable version every poll and never switched the symlink — the caller only ever saw 200 while the data plane kept running the old rules. Aligns the resource with `/ai-context-rules` and `/traffic-mirror-rules`, which already validate `cond` uniqueness before the write (issue #261).
- `PUT /open-api/v1/ai-cache-rules` no longer fails with `500 Database Exception: insert data not match` when the body mixes rules that carry `updated_at` (typically the `GET` readback) with rules that omit it. The mixed column set made gendry reject the bulk INSERT, and whether it failed depended on rule order — the natural "read, append, write back" edit path was blocked (issue #258).
- `POST /open-api/v1/batches/{batch_id}/cancel` no longer blocks on keyless providers: the cancel is allowed, the cancel intent is not rolled back, and cancel is idempotent under replay (issue #274).
- `batch_files` upsert now persists `filename`, fixing the missing `batch_files[].filename` key in task responses (issue #272).
- Batch task responses now emit zero/empty fields explicitly instead of omitting them (issue #271).
- Batch tasks now carry complete terminal fields plus backstop settlement parsing for the data plane's settle-on-download flow (issue #254).
- Batch management API contract: `product_name` removed from parameters and responses (internal field); queries still filter by the default product line (issue #276).
- `xreq` `BindForm`/`Bind`/`BindURI` type-conversion failures are now wrapped as 422 parameter errors instead of surfacing as 500 (issues #233, #237).
- The built-in admin password can now be changed through the API: name-referenced endpoints use `UserNameRef` (issue #226).
- DDL: `ai_cache_semantic_settings.updated_at` gains the missing `CURRENT_TIMESTAMP` default.
- Report: `go-sql-driver` upgraded to v1.9.3, root-fixing the NULL-bitmap bug (StarRocks FE binary-row package defect); the text protocol is now fixed as the query path.
- Report Doris: time-bucket `CAST` changed from `BIGINT` to `SIGNED` (compatible with real MySQL 8.4), and `LIMIT` is inlined as a literal (Doris does not support prepared placeholders).
- Report detail-table dedup key gains `logid`, aligning with the BFE logid rewrite (bfenetworks/bfe#1417).

## [0.0.10] - 2026-09-24

### Added
- Add report query API (`/open-api/v1/report/*`) with MySQL/Doris backends: cost/token report queries by model/provider/entity dimensions, plus optional aggregate job and partition management; add the report menu. The module assembles only with the optional `[Report]` + `[Databases.report_db]` config (endpoints stay 404 otherwise); backend tables see `db_ddl_report_mysql.sql`.
- Add optional `description` field (0-255 chars, no control characters) to the `/entities` API: supported on create, query, full update (omitting it clears the description) and partial update (omitted keeps original, explicit `""` clears); add the `description` column to the `entities` table (existing deployments: `ALTER TABLE entities ADD COLUMN description VARCHAR(255) NOT NULL DEFAULT ''`).
- Add per-protocol upstream base path config `protocol_paths` for providers (existing deployments: `ALTER TABLE providers ADD COLUMN protocol_paths JSON`).

### Changed
- Upgrade data-plane dependency `github.com/bfenetworks/bfe` to v1.8.8.
- Make `models` required for providers (at least one element) and remove the obsolete discover-models auto-fill description.
- Unify EPP pool group-size validation to 1~2 instances per group; remove the `EPPValidationMode` config item.
- Rate-limit rule rename now means delete old + add new; drop the name-immutable contract.

### Removed
- Remove the `/alb-pool` OpenAPI (GET detail + PATCH full-replace of the built-in BFE instance pool `BFE.aipool`): the pool data had no data-plane consumer; also remove the `RunTime.DefaultAIInstancePoolName` config item and the `alb_pool` integration tests. The dashboard AIInstancePool page loses its backend and will be cleaned up separately (breaking change).

### Fixed
- Mask API-Key plaintext in operation-log `error_msg` (mask at the source plus value-level redaction at the sink).
- Complete operation-log sensitive-field masking for token/provider credentials; fix nested quota-plan/rate-limit-policy audit completion and attribution.
- Build cluster audit `change_summary` in API vocabulary: lowercase keys at all nesting levels, API-aligned values (string `sticky_sessions.hash_strategy` enum, decoded `epp_config` object, `balance_mode` empty-value normalization) and trimmed internal bookkeeping fields (`id`/`ready`/`product_id`/`scheduler`/`sub_clusters`).
- Partial-update audits no longer materialize omitted fields as null, so `diff_keys` contains only actually-submitted fields (api_key/provider/cluster).
- Entity: reject PUT/PATCH carrying a different `type` (immutable after creation); create defaults omitted `allow_models` to `["*"]` and normalizes legacy `"[]"` rows on read.
- Model-prices: `GET /model-prices` with all three composite-key parameters returns the single record.
- Validate cluster `basic` numeric subfield ranges and `passive_health_check` value ranges and formats.
- EPP: compiled session-affinity configs pin `strategy=session_id`; `CompileEppConfig` explicitly distributes priority band 0 so a global `max_requests` is no longer silently truncated by hidden band defaults (5000/1GB).
- Report: normalize cost fields to amount (fixed-point ÷1e8) at the export boundary, fixing frontend display inflated by 1e8; timezone-neutral unix epoch rendering and partition boundary parsing; partition maintenance job queries `information_schema` by (schema, bare table name) so partitions are created when `[Report].Database` is configured.
- Quota: resetting an unlimited quota plan returns 422 Param Illegal instead of 500 Unknown Exception.
- API-Key: PUT full update validates `entity_id` existence, removing the asymmetry with PATCH.

## [0.0.9] - 2026-09-13

### Added
- Add operation log module: record create/update/delete operations including failed writes, with diff_keys, user_agent and real client_ip; add operation-log menu.
- Add EPP scheduling integration: distribute `balance_mode`, `epp_config`, EPP pools and assignments via unified `epp_data` export.
- Support Gemini provider protocol (model_protocols enum, `x-goog-api-key` auth header, discover URI mapping and models/ prefix stripping).
- Extend model prices: length-tier prices, 1h cache-write cost, image/audio token price keys; support scientific notation and >8-decimal precision prices.
- Add distributed lock for the quota reset scheduler.

### Changed
- Upgrade data-plane dependency `github.com/bfenetworks/bfe` to v1.8.7 (with `bfe-access-pb` v0.3.6 and `go-lib` v0.0.4).
- Bump `golang.org/x/crypto` from 0.32.0 to 0.45.0.
- Make exported config version numbers strictly monotonic per topic to avoid same-second version collisions stalling version advancement.
- Align API-Key route table type with the API documentation.

### Fixed
- Fix DAO statements not bound to the real transaction: updates took effect while the API returned 409.
- Fix entity update failure audit log losing resource identity.
- Fix PATCH semantics: omitted `models`/`subnet` on API keys no longer reset to `["*"]`; omitted `allow_models`/`block_models` on entities keep original values; PATCH /providers partial update keeps original values for omitted fields.
- Fix model-prices PUT partial update to merge `prices`/`tier_prices` by key; document merge-import whole-row overwrite contract.
- Fix DELETE model-prices/providers returning `Data: null` to match the API contract.
- Fix entity name charset to allow `@` (username@project form); allocate entity IDs from a sequence table to fix concurrent conflicts and ID reuse after deletion.
- Fix cert_name charset whitelist so certificates containing `/` can be deleted.
- Fix single-character cluster name lookup/delete wrongly rejected by the URI `min=2` parameter.
- Fix resource dependency conflicts returning 500/422 instead of 409.
- Fix operation-log pagination returning total 0 on the second page; log global route table updates.
- Add copyright headers to all Go files under `test/`.

## [0.0.8] - 2026-08-29

### Added
- Separate provider and cluster concepts; add provider management menu and endpoints.
- Support Anthropic Claude protocol forwarding in control plane.
- Support RMB tiered pricing (peak/off-peak tiers) for model prices.
- Support cluster-level session Key affinity (`llm_config.key_affinity`) configuration.
- Add `GET /providers/actions/get-provider-names` endpoint.
- Recognize `req_body_larger_than` and `req_body_less_than` in route expression verification.
- Add regression test support for quota reset periods with injectable Clock and BalanceSyncer.
- Replace mock Redis with miniredis in integration quota tests.

### Changed
- Remove `quota_balances` table; read quota balance directly from Redis.
- Simplify mod-api-key exported fields and extend ApikeyTag with TagLevel.
- Make discover-models a stateless tool endpoint.
- Batch preload entity/quota/entity-type data for mod-api-key export performance.
- Default provider instance name to address when omitted.
- Default `key_affinity.enabled` to true.
- Relax provider reference in model prices and add get-providers endpoint.

### Fixed
- Fix provider-cluster reference integrity for keys/models on update (#106).
- Sync provider `instance_pool` changes to referencing clusters (#106).
- Remove `instance_pool` name field and use `addr_port` for BFE backend name.
- Reject `name` in provider PATCH body and document deletion reference check.
- Preserve 8-decimal precision in model-prices JSON serialization (#102).
- Atomic API-Key sequence allocation to avoid MySQL lock wait timeout (#99).
- Atomic `api-key-{seq}` id allocation for concurrent creation (#80).
- Rename `providers.keys` to `api_keys` to avoid MySQL reserved word conflict.
- Instance_pool uniqueness now uses (name, addr, port).
- Return all providers when page/page_size are omitted in GET /providers.
- Ensure GET /providers/{name} returns time_zone and tiers.
- Remove gemini from initial model_protocols enum.
- Align entity name validation with entity-type (#81).
- Stable Redis key for RPM/TPM rate-limit rules (#84).
- Redis key cleanup on API-Key/Entity delete or rate-limit rule removal.
- Move quota_plan change balance adjustment down to QuotaPlanManager.


## [0.0.7] - 2026-08-20

### Added
- Multi-API-Key support for clusters (`Keys` + `KeyPolicy`) with integration tests.
- Provider/Model prefix routing support.
- Model pricing (model-price) menu and endpoints.
- Route rule `Cond` expression syntax validation.
- OpenAPI/InnerAPI schema integration tests.
- Centralized Redis quota operations in `model/quotacache`.
- Export `key_id` field in API-Key tokens.
- Added `AGENTS.md` for AI agent collaboration guidelines.

### Changed
- Rebranded organization migration: `infinity-ai-gateway` → `rainway-ai-gateway`.
- Unified RMB quota conversion using `github.com/bfenetworks/go-lib/quota`.
- Removed redundant `Currency` from exported `QuotaPlan` and updated InnerAPI docs.
- Normalized route rule field names to `snake_case`.
- Switched `cluster_conf` to native BFE `cluster_conf.AIConf`.
- Restructured `model` and `storage/rdb` package layouts to align with each other.

### Fixed
- `/api-keys/{id}/quota-plan` now returns the real quota balance.
- Prevent deletion of clusters/models referenced by route rules.
- Scan all route rules when checking cluster/model deletion references (#57).
- Validate model-pricing `limits` values as non-negative integers (#66).
- Various fixes for quota, Redis sync, and OpenAPI response details.

## [0.0.6] - 2026-08-05

### Added
- OpenAPI v0.3.0 upgrade: converged OpenAPI interfaces to API design v0.3.0.
- New endpoint `GET /route-tables` for route table listing.
- OpenAPI request parameter validation across endpoints.
- Auto-creation of the global route table during system initialization.
- Unit tests for `stateful`, `storage`, and all `model` packages with CI coverage gate.
- Integration test fixtures for `model_provider_type` and tool endpoints.

### Changed
- Refactored cluster model: aligned `instance_pool` model and related endpoints.
- Restructured test directory layout and updated integration test infrastructure.
- Split OpenAPI and InnerAPI design documents by module.
- Updated Chinese API design documents to v0.3.0.

### Fixed
- Fixed OpenAPI authorization flags: use `iauth.FA` instead of `iauth.FAP` for `/api-keys` and `/clusters` endpoints.
- Fixed concurrent map write in APIKey export.
- Fixed InnerAPI export issues for quota, routing, and rate-limiting policies.
- Fixed cluster deletion to check references from route rules.
- Fixed `sticky_sessions` default value to `CLIENT_IP_ONLY` and added validation for `hash_header` when `enabled=true`.
- Fixed API-Key, cluster, and AI-route issues (#42, #43, #44).
- Fixed integration tests: return JSON 404 for unmatched API paths and repaired failing test infrastructure.

## [0.0.4] - 2026-07-07

### Added
- Inner-API export endpoint `/inner-api/v1/configs/mod-body-process` for mod_body_process configuration with version management.
- Redis key existence check and creation logic in Entity update (PATCH) and full update (PUT) endpoints, ensuring quota keys are properly initialized.
- Redis key initialization in Entity create endpoint: when quota plan is unlimited, Redis key is initialized or reset to default value 100000000.

### Changed
- Optimized `allow_models` logic in exporter: token file Models is set to intersection of non-empty, non-* model lists from API-Key and all associated entities; if intersection is empty, Models is empty and Enabled is set to 2.
- Entity list interface uses BindForm for query parameters instead of requiring body parameters.
- Entity update interface supports PATCH (partial update) and PUT (full update) methods.

### Fixed
- API-Key create/update/full_update endpoints now return the created/updated API-Key data instead of nil.
- API-Key list endpoint returns proper format with `list` and `pagination`, supports filtering by `enabled`, `entity_id`, `unlimited_quota`.
- API-Key detail endpoint returns 404 when API-Key does not exist.
- API-Key `Enable` field JSON tag changed from `enable` to `enabled`, `UpdatedTime` from `updated_time` to `update_time`.
- Entity create endpoint adds required validation for `name` and existence check for `type`.
- Entity detail endpoint returns 404 when Entity does not exist.
- Entity delete endpoint checks for child entities and associated API-Keys before deletion.
- Entity list endpoint returns complete fields including `allow_models`, `block_models`, `quota_plan`, `rate_limit_policy`, `create_time`, `update_time`.
- Entity-Type create/update/list/one/delete endpoints fixed for proper return values and error handling.
- Reset quota endpoints return consistent format with `id`, `previous_quota`, `new_quota`, `balance`.
- HTTP status code handling: business errors (400-599) now return HTTP 200 with `ErrNum` field for business error code.
- Nil pointer dereference fixes in Entity and Entity-Type managers.
- Duplicate name/type_name detection with proper error codes.

## [0.0.3] - 2026-05-20

### Added
- **Quota Control & Rate Limiting**:
  - API-Key quota management with configurable quota amount, reset periods, and balance tracking.
  - API-Key rate limiting: TPM (Token Per Minute), RPM (Request Per Minute), and maximum concurrency limits.
  - Entity management with hierarchical structures (department/team/person), quota and rate limiting policies.
  - Entity-Type management with 1-5 level hierarchy relationships.
  - Quota balance synchronization: Redis-to-database synchronization with scheduled and manual reset support.
  - Unlimited quota mode for both API-Keys and Entities.

- **OpenAPI v1 Endpoints**:
  - `/api-keys`: Full CRUD with quota plan and rate limiting policy configuration.
  - `/api-keys/{id}/quota-plan`: Query API-Key quota plan with real-time balance.
  - `/api-keys/{id}/quota-plan/reset`: Reset API-Key quota balance.
  - `/entity-types`: Entity type CRUD endpoints.
  - `/entities`: Entity CRUD endpoints with hierarchical structure support.
  - `/entities/{id}/quota-plan`: Query Entity quota plan with real-time balance.
  - `/entities/{id}/quota-plan/reset`: Reset Entity quota balance.
  - `/models`: Get supported AI service model list.

- **Inner-API Endpoints**:
  - `/configs/mod-api-key`: Export API-Key configuration for BFE data-plane nodes.
  - `/configs/rate-limit-policy`: Export rate limiting policies for BFE enforcement.

- **Observability & Stability**:
  - Exception logger capturing panic information from main program, scheduled tasks, and HTTP handlers.
  - Global panic recovery mechanism.

- **Data Models**:
  - `QuotaPlan`: quota plan with unlimited, pass_when_no_enough_quota, quota, unit, reset_period fields.
  - `RateLimitPolicy`: rate limiting policy with enabled, rules (tpm, rpm, max_concurrency) fields.
  - `Entity`: entity with name, type, parent_id, allow_models, block_models fields.
  - `EntityType`: entity type with type_name, description, level fields.

## [0.0.1] - 2026-02-13

### Added
- First public release of AI Gateway API (control-plane component).
- OpenAPI v1 for managing gateway policies/configurations (products, clusters/subclusters, pools, domains, certificates, routes/forward rules, traffic scheduling, auth).
- AI route rules management for multi-model routing.
- API key management (enable/disable, quota, expiry, allowed models/subnets).
- Export endpoints for data-plane/Conf Agent configuration distribution.
- Built-in logging and Prometheus metrics endpoint on monitor port.
