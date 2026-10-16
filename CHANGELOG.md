<!--
This changelog should always be read on `master` branch. Its contents on other branches
does not necessarily reflect the changes.
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Changed
- Tighten `PATCH /epp-pool` group-size validation to exactly 1 instance per group (backup/standby pairs are no longer accepted at the API write path).
- Remove the `standby` field from the OpenAPI `/epp-assignments` responses (GET view and PUT override). The internal model view, the InnerAPI `epp_data` export and the `EPPAddr` semantics are unchanged (single-instance groups export `standby: null` / single-element `EPPAddr` as before; existing two-instance groups keep their read/export behavior). See `design-docs/modifications/2026-10-14-epp-openapi-single-instance/`.

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
