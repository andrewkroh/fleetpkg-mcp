# fleetpkg-mcp

`fleetpkg-mcp` is a Model Context Protocol (MCP) server that enables
LLMs to query metadata about Elastic Fleet integration packages.
It loads metadata from a local copy of the `elastic/integrations` repository
into a SQLite database and exposes query capabilities through the
Model Context Protocol.

## Features

- Indexes all Elastic Fleet packages (integration, input, and content) from your local `elastic/integrations` repository
- Creates a queryable SQLite database with comprehensive package metadata including fields, pipelines, transforms, variables, and test configurations
- Full-text search over package documentation, changelog entries, security detection rules, and ECS field definitions using SQLite FTS5 with porter stemming
- ECS field discovery and matching — search ~1990 ECS fields by concept or check field names against ECS to identify `external: ecs` candidates
- Exposes seven MCP tools: schema discovery, arbitrary SQL queries, doc search, changelog search, security rule search, ECS field search, and ECS field matching
- Periodic background refresh with optional `git pull` to keep data current
- Kubernetes-ready with `/healthz` and `/readyz` health check endpoints

## Installation

Requires [Go](https://go.dev/dl/). No install step needed — MCP clients run it directly with `go run`.

### Docker

The Docker image includes git and automatically clones the integrations repository on first run. It listens on HTTP port 8080 and refreshes every 24 hours by default:

```bash
docker run -p 8080:8080 -v fleetpkg-data:/data \
  -e FLEETPKG_MCP_REFRESH_INTERVAL=1h \
  ghcr.io/andrewkroh/fleetpkg-mcp:latest
```

#### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `FLEETPKG_MCP_REFRESH_INTERVAL` | Duration between automatic database refreshes (e.g., `1h`, `30m`, `24h`). Set to empty string to disable. | `24h` |
| `FLEETPKG_MCP_PPROF_ADDR` | Address for the pprof debug HTTP server (e.g., `0.0.0.0:6060`). Disabled if empty. | (disabled) |

## MCP Server Setup

The server requires a local checkout of the [elastic/integrations](https://github.com/elastic/integrations) repository.

### Claude Code / Claude Desktop

```bash
claude mcp add --scope user fleetpkg -- go run github.com/andrewkroh/fleetpkg-mcp@latest -dir /path/to/integrations
```

### Other MCP Clients

```json
{
  "mcpServers": {
    "fleetpkg": {
      "command": "go",
      "args": [
        "run",
        "github.com/andrewkroh/fleetpkg-mcp@latest",
        "-dir",
        "/path/to/integrations"
      ]
    }
  }
}
```

### HTTP Transport

For HTTP-based clients, start the server separately then point your client at the URL:

```bash
go run github.com/andrewkroh/fleetpkg-mcp@latest -dir /path/to/integrations -http 127.0.0.1:8080
```

## MCP Tools

| Tool | Description |
|------|-------------|
| `fleetpkg_get_sql_tables` | Returns the complete catalog of available tables and columns. Call this first. |
| `fleetpkg_execute_sql_query` | Executes an arbitrary read-only SQLite query. |
| `fleetpkg_search_docs` | Full-text search across package documentation. Supports FTS5 syntax: phrases, prefix matching, and boolean operators. |
| `fleetpkg_search_changelogs` | Full-text search across changelog entries. Same FTS5 syntax support. |
| `fleetpkg_search_security_rules` | Full-text search across security detection rules (title, description, query, setup, investigation notes). |
| `fleetpkg_search_ecs_fields` | Full-text search across ECS field definitions. Accepts plain keywords, dotted field names, or camelCase identifiers — automatically normalized for broad discovery. |
| `fleetpkg_match_ecs_fields` | Check whether field names exist in ECS. Returns each annotated with match status, data type, and description. |

## CLI Flags

### Required

| Flag | Description |
|------|-------------|
| `-dir <path>` | Path to your local checkout of the [elastic/integrations](https://github.com/elastic/integrations) repository |

### Optional

| Flag | Description |
|------|-------------|
| `-http <address>` | Listen for HTTP connections at the specified address instead of using stdin/stdout (e.g., `127.0.0.1:8080`) |
| `-pprof <address>` | Start a pprof debug HTTP server at the specified address (e.g., `127.0.0.1:6060`) |
| `-git-pull` | Clone the repository if missing, or `git pull --ff-only` if it exists. Updates before each periodic refresh |
| `-refresh <duration>` | Periodically refresh the database (e.g., `1h`, `30m`). Falls back to `FLEETPKG_MCP_REFRESH_INTERVAL` env var |
| `-log-level <level>` | Log level: `debug`, `info`, `warn`, `error` (default: `info`) |
| `-no-log` | Disable all logging output |
| `-version` | Print version information and exit |

## Database Schema

The database is built by [go-package-spec/pkgsql](https://github.com/andrewkroh/go-package-spec/tree/main/pkgsql), which reads packages using `pkgreader` and writes them into a self-documenting SQLite schema. For the complete schema, see [schema.sql](https://github.com/andrewkroh/go-package-spec/blob/main/pkgsql/internal/db/schema.sql).

### Tables

| Table | Description |
|-------|-------------|
| `packages` | Core metadata (name, version, type, description, ownership) for integration, input, and content packages |
| `policy_templates` | Configuration templates with deployment modes (default, agentless) |
| `policy_template_inputs` | Inputs defined within policy templates |
| `policy_template_categories` | Categories assigned to policy templates |
| `policy_template_icons` | Icon definitions for policy templates |
| `policy_template_screenshots` | Screenshot definitions for policy templates |
| `data_streams` | Data streams with Elasticsearch and agent configuration |
| `streams` | Individual streams (inputs) within data streams |
| `agent_templates` | Agent Handlebars template files (.yml.hbs) with raw content |
| `fields` | Elasticsearch field definitions, flattened from nested YAML into dotted-path names with ECS resolution |
| `data_stream_fields` | Join table linking fields to data streams |
| `package_fields` | Join table linking fields to packages (for input packages) |
| `transform_fields` | Join table linking fields to transforms |
| `transforms` | Elasticsearch transform configurations with pivot, latest, source, and destination settings |
| `vars` | Input variable definitions with type, default value, and validation |
| `package_vars` | Join table linking vars to packages |
| `policy_template_vars` | Join table linking vars to policy templates |
| `policy_template_input_vars` | Join table linking vars to policy template inputs |
| `stream_vars` | Join table linking vars to streams |
| `ingest_pipelines` | Elasticsearch ingest pipeline definitions within data streams |
| `ingest_processors` | Individual processors flattened from pipelines, including nested on_failure handlers |
| `kibana_saved_objects` | Kibana saved objects (dashboards, visualizations, security rules, etc.) from the `kibana/` directory |
| `kibana_references` | References between Kibana saved objects, enabling dependency graph queries |
| `security_rules` | Security detection rule attributes (query, severity, risk score, MITRE mappings) extracted from Kibana saved objects |
| `security_rule_index_patterns` | Elasticsearch index patterns monitored by security rules |
| `security_rule_tags` | Structured tags on security rules (Domain, OS, Tactic, Data Source) |
| `security_rule_threats` | MITRE ATT&CK tactic and technique mappings for security rules |
| `security_rule_related_integrations` | Integration packages related to security rules |
| `security_rule_required_fields` | Fields required by security rules |
| `security_rules_fts` | FTS5 full-text search index over security rule title, description, query, setup, and notes |
| `sample_events` | Example event data for data streams |
| `images` | Image file metadata (dimensions, size, SHA-256) from the `img/` directory |
| `package_icons` | Icon definitions for packages |
| `package_screenshots` | Screenshot definitions for packages |
| `docs` | Documentation files (READMEs, guides, knowledge base articles) with optional content |
| `docs_fts` | FTS5 full-text search index over doc content |
| `changelogs` | Changelog versions with release dates (via git blame) |
| `changelog_entries` | Individual changelog entries with description, type, and link |
| `changelog_entries_fts` | FTS5 full-text search index over changelog descriptions |
| `package_categories` | Categories assigned to packages |
| `discovery_fields` | Package discovery capability metadata |
| `build_manifests` | Build configuration and ECS dependencies |
| `tags` | Kibana tags associated with packages |
| `deprecations` | Deprecation notices for packages, policy templates, inputs, data streams, and vars |
| `ecs_fields` | ECS (Elastic Common Schema) field definitions from the latest version (~1990 fields) with name, data type, description, and FTS5 search text |
| `ecs_fields_fts` | FTS5 full-text search index over ECS field names (tokenized) and descriptions |
| `routing_rules` | Routing rules for rerouting documents from a source dataset |
| `pipeline_tests` | Pipeline test cases for data streams |
| `system_tests` | System test cases for data streams and input packages |
| `static_tests` | Static test cases for data streams |
| `policy_tests` | Policy test cases for data streams and input packages |

## Example Conversations

> **Q**: Use the fleetpkg tools to answer: What integrations configure a pivot type transform?
>
> **A**: The following integrations configure a pivot type transform:
>
> * aws_billing
> * beaconing
> * ded
> * elasticsearch
> * lmd
> * pad

> **Q**: Of those, which do not have an ingest node pipeline associated with the transform?
>
> **A**: The integrations that configure a pivot type transform and do not have an ingest node pipeline associated with the transform are:
>
> * lmd
> * pad

> **Q**: What github team owns those two integrations?
>
> **A**: The `elastic/sec-applied-ml` GitHub team owns both the `lmd` and `pad` integrations.

----

> **Q**: Use the fleetpkg tools to answer the following question. What integration
> data streams define a 'resource' field? And what are the data types of the
> fields?
>
> **A**: The following integration data streams define a 'resource' field of type 'keyword':
>
> - endace > flow
> - network_traffic > amqp
> - network_traffic > cassandra
> - network_traffic > dhcpv4
> - network_traffic > dns
> - network_traffic > flow
> - network_traffic > http
> - network_traffic > icmp
> - network_traffic > memcached
> - network_traffic > mongodb
> - network_traffic > mysql
> - network_traffic > nfs
> - network_traffic > pgsql
> - network_traffic > redis
> - network_traffic > sip
> - network_traffic > thrift
> - network_traffic > tls

----

> **Q**: Analyze the screenshots to determine the percentage of screenshots that
> have the correct 'size' column in comparison to the computed width and height
> column values. Include the total numbers as well.
>
> *A*: Based on my analysis of the screenshots in the database, here are the results:
>
> Summary Statistics
>
> - Total screenshots: 932
> - Correct size values: 169 (18.13%)
> - Incorrect size values: 763 (81.87%)
>
> Analysis
>
> Only 18.13% of screenshots have the correct 'size' column value that matches the computed dimensions from the width and height columns. This means that 81.87% of the screenshots have mismatched or incorrect size specifications.
>
> Common Issues Found
>
> Looking at the sample data, I can see several types of mismatches:
>
> 1. Placeholder values: Many screenshots have size="600x600" when the actual dimensions are different (e.g., 1920x1080, 1899x2069, 1920x1233)
> 2. Scaled/resized values: Screenshots show evidence of being resized - the size column contains smaller dimensions than the actual image dimensions (e.g., size shows 1896x961 but actual is 2880x1800)
> 3. Character encoding issues: At least one case shows size="1608x2692" (using x instead of x) while computed is "1608x2692"
> 4. Minor width discrepancies: Some have small differences in width (e.g., size="1280x1329" vs computed "1268x1329")

----

> **Q**: Use the fleetpkg MCP tool. Analyze the number of 'set' ingest
> processors that use the 'copy_from' attribute vs the 'value' attribute with a
> Mustache template value matching '{{%}}'
>
> **A**:
> Here are the results of the analysis of 'set' ingest processors:
> - Number of processors using the `copy_from` attribute: 6992
> - Number of processors using the `value` attribute with a Mustache template:
    1174
>
> **Q**: What are the top 5 teams responsible for the packages using 'set' with
> a Mustache template instead of 'copy_from'?
>
> **A**:
> Here are the top 5 teams using 'set' with a Mustache template instead of 'copy_from':
>
> 1. elastic/security-service-integrations: 395 uses
> 2. elastic/integration-experience: 321 uses
> 3. elastic/obs-infraobs-integrations: 245 uses
> 4. elastic/obs-ds-hosted-services: 100 uses
> 5. elastic/elastic-agent-data-plane: 36 uses

## License

This project is licensed under the Apache License 2.0 - see the LICENSE.txt file for details.
