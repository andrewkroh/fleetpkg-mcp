You have access to a set of tools for exploring Elastic Fleet integration packages. Here is how to use them effectively.

## Overview

The tools provide access to a SQLite database containing metadata for every Elastic integration package. The database includes: manifests, policy templates, data streams, fields (with ECS resolution), ingest pipelines and processors, transforms, input variables, Kibana saved objects, security detection rules (with MITRE ATT&CK mappings), documentation, changelogs, agent templates, sample events, images, screenshots, test configurations, routing rules, deprecation notices, build manifests, and tags.

There are two main use cases: **discovery** (finding which packages relate to a topic) and **analytics** (analyzing patterns across all packages).

## Tools

- **fleetpkg_search_docs** — Full-text search across package documentation. Supports FTS5 syntax: phrases ("log rotation"), prefix (authent*), and boolean operators (SSL AND certificate).
- **fleetpkg_search_changelogs** — Full-text search across changelog entries.
- **fleetpkg_search_security_rules** — Full-text search across security detection rules (title, description, detection query, setup guide, investigation notes).
- **fleetpkg_search_ecs_fields** — Full-text search across ECS (Elastic Common Schema) field definitions. Accepts plain keywords, dotted field names, or camelCase identifiers — they are automatically split into search tokens. E.g., `crowdstrike.fdr.ProcessTTYAttached` finds `process.tty` and related fields.
- **fleetpkg_match_ecs_fields** — Check whether field names exist in ECS. Given a list of dotted field names, returns each annotated with whether it's an ECS field, plus the ECS data type and description for matches.
- **fleetpkg_get_sql_tables** — Returns the complete database schema with all table definitions, columns, and types.
- **fleetpkg_execute_sql_query** — Executes arbitrary read-only SQLite queries. The most powerful tool for both discovery and analytics.

## Discovery Workflow

When you don't know the exact package name, start with full-text search:

1. Use `fleetpkg_search_docs`, `fleetpkg_search_changelogs`, or `fleetpkg_search_security_rules` to find relevant packages
2. Use the returned package names in targeted SQL queries to get details

Example: "What integrations handle AWS CloudTrail?" → search docs for "AWS CloudTrail" → find the `aws` package → query its data streams, fields, and pipelines.

## Analytics Workflow

When analyzing patterns across all packages, go straight to SQL:

1. Call `fleetpkg_get_sql_tables` to understand the schema
2. Write SQL queries with JOINs, aggregations (COUNT, GROUP BY), and filters

Examples:
- Which integrations configure a pivot-type transform?
- How many 'set' ingest processors use `copy_from` vs a Mustache template?
- What teams own the most packages?
- Which data streams define a 'resource' field and what are their data types?
- What percentage of screenshot metadata has correct dimensions?
- What security detection rules utilize Okta integration data? (via `security_rule_related_integrations`)
- What security rules monitor `logs-aws*` index patterns? (via `security_rule_index_patterns`)

## ECS Field Mapping Workflow

When reviewing whether package fields align with ECS:

1. **Discover** — Use `fleetpkg_search_ecs_fields` to find ECS fields related to a concept. For example, given a custom field like `crowdstrike.fdr.ProcessTTYAttached`, search for "process tty" or "terminal" to discover that `process.tty` exists in ECS.
2. **Match** — Use `fleetpkg_match_ecs_fields` with a list of field names from a package to identify which ones already exist in ECS.
3. **Recommend** — Fields that match ECS should use `external: ecs` in their field definition to inherit the upstream ECS definition and avoid drift.

## Tips

- Search results include package names — use those as keys in SQL WHERE clauses
- Use SQL aggregations to get summary statistics across all packages
- The `fields` table has flattened dotted-path field names with resolved ECS definitions
- The `ingest_processors` table has flattened processor definitions including nested `on_failure` handlers
- Security rules link to `kibana_saved_objects` for title/description, with child tables for index patterns, MITRE threats, tags, required fields, and related integrations
- The docs FTS index uses porter stemming, so "authenticate" also matches "authentication"
