## v0.14.0

### Package Spec Support

- Updated `go-package-spec` to expose the Elastic **Agent version condition** ([go-package-spec#24](https://github.com/andrewkroh/go-package-spec/pull/24)): integration and input manifests can restrict which Agent versions may run the package via `conditions.agent.version`. This was previously read but silently dropped; it's now stored in a new `conditions_agent_version` column on the `packages` table, alongside the existing `conditions_kibana_version` and `conditions_elastic_subscription` columns.

### Dependency Updates

- Bumped `github.com/andrewkroh/go-package-spec` to pick up the Agent version condition support. (#68)

**Full Changelog**: https://github.com/andrewkroh/fleetpkg-mcp/compare/v0.13.0...v0.14.0

---

## v0.13.0

### Package Spec Support

- Updated `go-package-spec` to add support for **package-spec 3.6.4 and 3.6.5** ([go-package-spec#23](https://github.com/andrewkroh/go-package-spec/pull/23)):
  - **Variable migration** — New `migrate_from` object on variables (`name`, `scope`, `stream`) that records how a variable was previously named/scoped so Fleet can migrate existing policies. The `vars` table gains a `migrate_from` JSON column.
  - **Provider permissions** — New `provider_permissions` field on integration manifests and policy templates, stored as `[]ProviderPermission`.
  - **Data stream type rename** — The `DataStreamType` enum value `profiling` was renamed to `profiles`.
  - **Type fidelity** — Restored strongly-typed `[]Var` for `DataStreamStream.Vars` and `PolicyTemplateInput.Vars`, which a schema restructuring had degraded to `[]map[string]any`. (#66)

### Go Toolchain

- Moved to **Go 1.26** — `go.mod` now requires `go 1.26.4`, and CI uses `go-version: stable` so tests always run against the latest stable Go release. (#66)

### Security

- Resolved all `govulncheck` findings (17 → 0). Standard-library advisories (net/textproto, crypto/x509, html/template, net, crypto/tls, net/url, os, net/http) are addressed by the Go 1.26 upgrade, and the module advisories GO-2026-4985 (OpenTelemetry OTLP HTTP exporters) and GO-2026-4918 (`golang.org/x/net`) are fixed by the dependency bumps below. (#66)

### Dependency Updates

- Bumped `github.com/andrewkroh/go-package-spec` to pick up package-spec 3.6.5 support. (#66)
- Bumped OpenTelemetry to **v1.44.0** (OTLP exporters v1.44.0, log exporters v0.20.0), including `go.opentelemetry.io/contrib/exporters/autoexport` from v0.65.0 to **v0.69.0**. (#66, #63)
- Bumped `golang.org/x/net` to **v0.56.0**. (#66)
- Bumped `modernc.org/sqlite` from v1.48.0 to **v1.53.0**. (#66, #64)

**Full Changelog**: https://github.com/andrewkroh/fleetpkg-mcp/compare/v0.12.0...v0.13.0

---

## v0.12.0

### Package Spec Support

- Updated `go-package-spec` to add support for **package-spec 3.6.3**, which introduces a new `release` field on `deployment_modes.agentless` in policy templates. This field controls the maturity level of the agentless deployment mode (`beta` or `ga`). When omitted, Kibana derives a default from the agentless platform's own maturity. Packages where agentless is the only deployment mode should defer to the package's top-level `version`. Only evaluated in Kibana 9.5.0+. The `policy_templates` table gains a new `deployment_modes_agentless_release` column. (#62)

### Dependency Updates

- Bumped `github.com/andrewkroh/go-package-spec` to pick up package-spec 3.6.3 support. (#62)
- Bumped `github.com/modelcontextprotocol/go-sdk` from v1.4.1 to **v1.6.0** (#60)
- Bumped `go.opentelemetry.io/contrib/instrumentation/host` from v0.65.0 to **v0.68.0** (#61)

---

## v0.11.0

### New Features

- **Nested package layout support** — The `elastic/integrations` repository now allows packages to live under nested subdirectories (e.g. `packages/<group>/<name>/`) in addition to the existing flat `packages/<name>/` layout. Database build now uses `pkgreader.ListPackages` to walk both layouts, validating each `manifest.yml` (`format_version`, `name`, `type`, `version`) before treating a directory as a package. Each package's repo-relative path is computed with `filepath.Rel` and passed via `WithPathPrefix`, so `file_path` columns in the SQLite database always reflect the full repo-relative location and CODEOWNERS lookups resolve correctly for nested packages. (#59)

- **Package-spec 3.6.2 support** — Updated `go-package-spec` to add support for **package-spec 3.6.2**, which introduces a number of new spec features and corresponding database schema changes ([go-package-spec#18](https://github.com/andrewkroh/go-package-spec/pull/18)):
  - **Sections** — New `Section{Name, Title, Description}` type, attached to integration manifests, input manifests, policy templates, policy template inputs, and data stream streams. Backed by a new `sections` SQLite table.
  - **Var groups** — Existing var groups are now first-class with a `show_divider` field and stored in new `var_groups` and `var_group_options` SQLite tables. `vars` gains a `section` column referencing a sibling section.
  - **Multiple policy template inputs of the same type** — `policy_template_inputs` gains `name` and `show_divider` columns so multiple inputs sharing a type can coexist.
  - **Named sample events** — `sample_event_<name>.json` files are now cataloged alongside the unnamed `sample_event.json`. `sample_events` gains a nullable `name` column.
  - **System test samples** — New `system_test_samples` table catalogs `sample_event_<name>.json` references with their match conditions.
  - **Schema relationalization** — Sections, var groups, and samples that previously would have been stored as opaque JSON columns on `policy_templates`, `policy_template_inputs`, `streams`, and `system_tests` are now stored in dedicated tables with multi-parent foreign keys, so consumers can query them relationally.
  - **Field type** — `FieldType` adds the `geo_shape` enum value.
  - **Transform settings** — `TransformSettings` gains `num_failure_retries`.

### Dependency Updates

- Bumped `github.com/andrewkroh/go-package-spec` to pick up package-spec 3.6.2 support and the nested-layout / CODEOWNERS fix. (#59)
- Bumped `modernc.org/sqlite` from v1.46.2 to **v1.48.0** (#57)
- Bumped `go.opentelemetry.io/otel`, `otel/metric`, `otel/sdk`, and `otel/sdk/metric` from v1.41.0/v1.42.0 to **v1.43.0** (#56)

---

## v0.10.1

### Bug Fixes

- **Stateless MCP transport** — Switched from a stateful session with a 1-hour idle timeout to stateless mode. All tools are read-only and the server never sends client-bound requests, so session state is unnecessary. This eliminates session timeout failures that caused tools to stop working after idle periods, and prevents goroutine leaks from abandoned sessions. Clients can now sit idle indefinitely between tool calls. (#55)

### Dependency Updates

- Bumped `github.com/modelcontextprotocol/go-sdk` from v1.4.0 to **v1.4.1** — fixes [CVE-2026-33252](https://github.com/modelcontextprotocol/go-sdk/security/advisories) (CSRF / cross-site tool execution via missing Origin validation) (#55)
- Bumped `go.opentelemetry.io/otel/metric` from v1.41.0 to **v1.42.0** (#52)
- Bumped `modernc.org/sqlite` from v1.46.1 to **v1.46.2** (#51)
- Bumped `docker/login-action` from 3 to **4** (#54)

---

## v0.10.0

### Package Spec Support

- Updated `go-package-spec` to add support for **package-spec 3.5.8**.

### Dependency Updates

- Bumped `github.com/modelcontextprotocol/go-sdk` from v1.3.1 to **v1.4.0** (#46)
- Bumped `go.opentelemetry.io/contrib/instrumentation/runtime` from v0.65.0 to **v0.66.0** (#47)
- Bumped `go.opentelemetry.io/otel` and related packages from v1.40.0 to **v1.41.0**

---

## v0.9.0

### New Features

- **CODEOWNERS enrichment** — Data streams are now enriched with `github_code_owner`, populated from the integrations repository's `.github/CODEOWNERS` file. This lets you query which GitHub team owns each data stream. (#45)

### Bug Fixes

- **FTS dot-syntax errors** — Dotted field names like `source.nat.ip` passed to FTS search tools no longer cause `syntax error near '.'`. A shared `sanitizeFTSQuery` helper now replaces dots with spaces across all four FTS search tools (`fleetpkg_search_docs`, `fleetpkg_search_changelogs`, `fleetpkg_search_security_rules`, `fleetpkg_search_ecs_fields`).

- **MCP session timeout** — Increased the MCP session timeout from 10 minutes to 1 hour to prevent sessions from expiring during long-running interactions.

### Improvements

- **Match tool logging** — `fleetpkg_match_ecs_fields` now logs the `field_names` input for better query visibility and debugging.

---

## v0.8.0

### New Features

- **ECS field search** — Added `fleetpkg_search_ecs_fields`, a new MCP tool for full-text search across ~1990 ECS (Elastic Common Schema) field definitions. Queries are automatically normalized: dotted field names are split on `.`, camelCase identifiers are split into words, and plain terms are OR-joined for broad discovery ranking. For example, querying `crowdstrike.fdr.ProcessTTYAttached` finds `process.tty` and related fields. (#44)

- **ECS field matching** — Added `fleetpkg_match_ecs_fields`, a new MCP tool that checks whether field names exist in ECS. Accepts up to 500 dotted field names and returns each annotated with `is_ecs`, `ecs_data_type`, and `ecs_description`. Use this to identify which package fields should use `external: ecs` to inherit the upstream ECS definition. (#44)

- **ECS fields database table** — ECS field definitions from the latest version are now loaded into an `ecs_fields` SQLite table with an `ecs_fields_fts` FTS5 index during database initialization. The data is sourced from the `go-ecs` library with no runtime network calls. (#44)

### Improvements

- **Query logging for search tools** — All FTS search tools (`fleetpkg_search_docs`, `fleetpkg_search_changelogs`, `fleetpkg_search_security_rules`, `fleetpkg_search_ecs_fields`) now log the SQL statement and query arguments on success, matching the logging behavior of `fleetpkg_execute_sql_query`. (#44)

- **Reduced token usage** — FTS search tool results now omit NULL-valued columns from JSON output instead of returning `"column": null`, reducing token consumption for LLM consumers. This applies to all tools using the `queryJSON` helper; the raw `fleetpkg_execute_sql_query` tool preserves NULLs for schema fidelity. (#44)
