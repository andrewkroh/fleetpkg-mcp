// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/andrewkroh/fleetpkg-mcp/internal/otelsetup"
)

type tools struct {
	tables  []string
	db      *atomic.Pointer[sql.DB]
	log     *slog.Logger
	metrics *otelsetup.Metrics
}

func newTools(tables []string, db *atomic.Pointer[sql.DB], log *slog.Logger, metrics *otelsetup.Metrics) *tools {
	return &tools{
		tables:  tables,
		db:      db,
		log:     log,
		metrics: metrics,
	}
}

func AddTools(s *mcp.Server, tables []string, db *atomic.Pointer[sql.DB], log *slog.Logger, metrics *otelsetup.Metrics) {
	t := newTools(tables, db, log, metrics)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "fleetpkg_get_sql_tables",
		Description: `Call this tool first! Returns the complete catalog of available tables and columns.`,
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
			ReadOnlyHint:   true,
		},
	}, t.getSQLTables)

	mcp.AddTool(s, &mcp.Tool{
		Name: "fleetpkg_execute_sql_query",
		Description: `Call this tool to execute an arbitrary SQLite query.
Be sure you have called fleetpkg_get_sql_tables() first to understand the structure of the data!`,
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
			ReadOnlyHint:   true,
		},
	}, t.executeQuery)

	mcp.AddTool(s, &mcp.Tool{
		Name: "fleetpkg_search_docs",
		Description: `Full-text search across package documentation (READMEs, guides, knowledge base articles).
Uses FTS5 with porter stemming — supports phrases ("log rotation"), prefix (authent*), and boolean operators (AND/OR/NOT).
Returns matching doc snippets with package name, doc type, and file path, sorted by relevance.`,
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
			ReadOnlyHint:   true,
		},
	}, t.searchDocs)

	mcp.AddTool(s, &mcp.Tool{
		Name: "fleetpkg_search_changelogs",
		Description: `Full-text search across package changelog entries.
Uses FTS5 with porter stemming — supports phrases ("fix bug"), prefix (SSL*), and boolean operators (AND/OR/NOT).
Returns matching changelog entries with package name, version, change type, description, and link, sorted by relevance.`,
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
			ReadOnlyHint:   true,
		},
	}, t.searchChangelogs)

	mcp.AddTool(s, &mcp.Tool{
		Name: "fleetpkg_search_security_rules",
		Description: `Full-text search across security detection rules (title, description, query, setup guide, investigation note).
Uses FTS5 with porter stemming — supports phrases ("credential access"), prefix (powershell*), and boolean operators (AND/OR/NOT).
Returns matching rules with package name, rule ID, type, severity, risk score, title, and description, sorted by relevance.`,
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
			ReadOnlyHint:   true,
		},
	}, t.searchSecurityRules)

	mcp.AddTool(s, &mcp.Tool{
		Name: "fleetpkg_search_ecs_fields",
		Description: `Full-text search across ECS (Elastic Common Schema) field definitions.
Use this to discover ECS fields related to a concept. Accepts plain keywords, dotted field names, or camelCase identifiers —
the query is automatically normalized (dots and camelCase are split into tokens, plain terms are OR-joined for broad discovery).
Example: "crowdstrike.fdr.ProcessTTYAttached" finds process.tty and related fields.
Also supports FTS5 syntax when needed: phrases ("source address"), prefix (authent*), and boolean operators (network AND bytes).
Returns matching fields with name, data_type, description, is_array, and pattern, sorted by relevance.`,
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
			ReadOnlyHint:   true,
		},
	}, t.searchECSFields)

	mcp.AddTool(s, &mcp.Tool{
		Name: "fleetpkg_match_ecs_fields",
		Description: `Check whether field names exist in ECS (Elastic Common Schema).
Given a list of dotted field names, returns each annotated with is_ecs (bool), and for matches: ecs_data_type and ecs_description.
Use this to identify which package fields should use "external: ecs" to inherit the upstream ECS definition.`,
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
			ReadOnlyHint:   true,
		},
	}, t.matchECSFields)
}

const (
	toolGetSQLTables        = "fleetpkg_get_sql_tables"
	toolExecuteSQLQuery     = "fleetpkg_execute_sql_query"
	toolSearchDocs          = "fleetpkg_search_docs"
	toolSearchChangelogs    = "fleetpkg_search_changelogs"
	toolSearchSecurityRules = "fleetpkg_search_security_rules"
	toolSearchECSFields     = "fleetpkg_search_ecs_fields"
	toolMatchECSFields      = "fleetpkg_match_ecs_fields"
)

func (t *tools) getSQLTables(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	start := time.Now()
	schemas := strings.Join(t.tables, "\n")
	t.metrics.RecordToolCall(ctx, toolGetSQLTables, true, time.Since(start))
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: schemas},
		},
	}, nil, nil
}

type ExecuteQueryArgs struct {
	Statement string `json:"statement" jsonschema:"SQLite query to execute"`
}

const queryTimeout = 10 * time.Second

func (t *tools) executeQuery(ctx context.Context, req *mcp.CallToolRequest, args ExecuteQueryArgs) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	start := time.Now()
	success := false
	defer func() {
		t.metrics.RecordToolCall(ctx, toolExecuteSQLQuery, success, time.Since(start))
	}()

	db := t.db.Load()
	if db == nil {
		t.log.WarnContext(ctx, "Query failed",
			slog.String("statement", args.Statement),
			slog.String("error", "database not ready"),
			slog.Duration("duration", time.Since(start)))
		t.metrics.RecordError(ctx, "db_not_ready")
		return mcpErrorf("database is still initializing, please retry in a moment"), nil, nil
	}

	rows, err := db.QueryContext(ctx, args.Statement)
	if err != nil {
		t.log.ErrorContext(ctx, "Query failed",
			slog.String("statement", args.Statement),
			slog.Any("error", err),
			slog.Duration("duration", time.Since(start)))
		t.metrics.RecordSQLQuery(ctx, false, time.Since(start))
		t.metrics.RecordError(ctx, "sql_query")
		return mcpErrorf("failed to execute query: %v", err), nil, nil
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		t.log.ErrorContext(ctx, "Query failed",
			slog.String("statement", args.Statement),
			slog.Any("error", err),
			slog.Duration("duration", time.Since(start)))
		t.metrics.RecordSQLQuery(ctx, false, time.Since(start))
		t.metrics.RecordError(ctx, "sql_columns")
		return mcpErrorf("failed to get columns: %v", err), nil, nil
	}

	var result []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}

		if err := rows.Scan(pointers...); err != nil {
			t.log.ErrorContext(ctx, "Query failed",
				slog.String("statement", args.Statement),
				slog.Any("error", err),
				slog.Duration("duration", time.Since(start)))
			t.metrics.RecordSQLQuery(ctx, false, time.Since(start))
			t.metrics.RecordError(ctx, "sql_scan")
			return mcpErrorf("failed to scan row: %v", err), nil, nil
		}

		row := make(map[string]interface{})
		for i, column := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[column] = string(b)
			} else {
				row[column] = val
			}
		}
		result = append(result, row)
	}

	jsonRows, err := json.Marshal(result)
	if err != nil {
		t.log.ErrorContext(ctx, "Query failed",
			slog.String("statement", args.Statement),
			slog.Any("error", err),
			slog.Duration("duration", time.Since(start)))
		t.metrics.RecordSQLQuery(ctx, false, time.Since(start))
		t.metrics.RecordError(ctx, "json_marshal")
		return mcpErrorf("failed to marshal result: %v", err), nil, nil
	}

	success = true
	t.metrics.RecordSQLQuery(ctx, true, time.Since(start))
	t.log.InfoContext(ctx, "Query executed",
		slog.String("statement", args.Statement),
		slog.Int("row_count", len(result)),
		slog.Duration("duration", time.Since(start)))
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonRows)},
		},
	}, nil, nil
}

const defaultFTSLimit = 20

type SearchDocsArgs struct {
	Query       string `json:"query" jsonschema:"FTS5 search query (e.g. authentication, \"log rotation\", SSL AND certificate)"`
	PackageName string `json:"package_name,omitempty" jsonschema:"Optional package name to restrict search to a single package"`
	Limit       int    `json:"limit,omitempty" jsonschema:"Maximum number of results to return (default 20)"`
}

func (t *tools) searchDocs(ctx context.Context, req *mcp.CallToolRequest, args SearchDocsArgs) (*mcp.CallToolResult, any, error) {
	limit := args.Limit
	if limit <= 0 {
		limit = defaultFTSLimit
	}

	stmt := `SELECT p.name AS package_name, d.content_type, d.file_path,
snippet(docs_fts, 0, '>>>', '<<<', '...', 48) AS snippet
FROM docs_fts
JOIN docs d ON d.id = docs_fts.rowid
JOIN packages p ON p.id = d.packages_id
WHERE docs_fts MATCH ?`

	queryArgs := []any{args.Query}
	if args.PackageName != "" {
		stmt += "\nAND p.name = ?"
		queryArgs = append(queryArgs, args.PackageName)
	}
	stmt += "\nORDER BY rank\nLIMIT ?"
	queryArgs = append(queryArgs, limit)

	return t.queryJSON(ctx, toolSearchDocs, stmt, queryArgs...)
}

type SearchChangelogsArgs struct {
	Query       string `json:"query" jsonschema:"FTS5 search query (e.g. SSL, \"bug fix\", deprecated AND removed)"`
	PackageName string `json:"package_name,omitempty" jsonschema:"Optional package name to restrict search to a single package"`
	Limit       int    `json:"limit,omitempty" jsonschema:"Maximum number of results to return (default 20)"`
}

func (t *tools) searchChangelogs(ctx context.Context, req *mcp.CallToolRequest, args SearchChangelogsArgs) (*mcp.CallToolResult, any, error) {
	limit := args.Limit
	if limit <= 0 {
		limit = defaultFTSLimit
	}

	stmt := `SELECT p.name AS package_name, cl.version, ce.type, ce.description, ce.link
FROM changelog_entries_fts
JOIN changelog_entries ce ON ce.id = changelog_entries_fts.rowid
JOIN changelogs cl ON cl.id = ce.changelogs_id
JOIN packages p ON p.id = cl.packages_id
WHERE changelog_entries_fts MATCH ?`

	queryArgs := []any{args.Query}
	if args.PackageName != "" {
		stmt += "\nAND p.name = ?"
		queryArgs = append(queryArgs, args.PackageName)
	}
	stmt += "\nORDER BY rank\nLIMIT ?"
	queryArgs = append(queryArgs, limit)

	return t.queryJSON(ctx, toolSearchChangelogs, stmt, queryArgs...)
}

type SearchSecurityRulesArgs struct {
	Query       string `json:"query" jsonschema:"FTS5 search query (e.g. \"credential access\", powershell*, persistence AND registry)"`
	PackageName string `json:"package_name,omitempty" jsonschema:"Optional package name to restrict search to a single package"`
	Limit       int    `json:"limit,omitempty" jsonschema:"Maximum number of results to return (default 20)"`
}

func (t *tools) searchSecurityRules(ctx context.Context, req *mcp.CallToolRequest, args SearchSecurityRulesArgs) (*mcp.CallToolResult, any, error) {
	limit := args.Limit
	if limit <= 0 {
		limit = defaultFTSLimit
	}

	stmt := `SELECT p.name AS package_name, sr.rule_id, sr.type, sr.severity, sr.risk_score,
kso.title, kso.description
FROM security_rules_fts
JOIN security_rules sr ON sr.id = security_rules_fts.rowid
JOIN kibana_saved_objects kso ON kso.id = sr.kibana_saved_objects_id
JOIN packages p ON p.id = kso.packages_id
WHERE security_rules_fts MATCH ?`

	queryArgs := []any{args.Query}
	if args.PackageName != "" {
		stmt += "\nAND p.name = ?"
		queryArgs = append(queryArgs, args.PackageName)
	}
	stmt += "\nGROUP BY sr.rule_id\nORDER BY rank\nLIMIT ?"
	queryArgs = append(queryArgs, limit)

	return t.queryJSON(ctx, toolSearchSecurityRules, stmt, queryArgs...)
}

type SearchECSFieldsArgs struct {
	Query string `json:"query" jsonschema:"Search query — plain keywords, dotted field names, or camelCase identifiers (e.g. process terminal, crowdstrike.fdr.ProcessTTYAttached, \"source address\", network AND bytes)"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of results to return (default 20)"`
}

func (t *tools) searchECSFields(ctx context.Context, req *mcp.CallToolRequest, args SearchECSFieldsArgs) (*mcp.CallToolResult, any, error) {
	limit := args.Limit
	if limit <= 0 {
		limit = defaultFTSLimit
	}

	// Normalize the query for FTS5 matching:
	// 1. Replace dots with spaces (e.g. "crowdstrike.fdr.Name" → "crowdstrike fdr Name")
	// 2. Split camelCase tokens (e.g. "ProcessTTYAttached" → "Process TTY Attached")
	// 3. Join plain terms with OR for additive discovery ranking
	query := strings.ReplaceAll(args.Query, ".", " ")
	query = splitCamelCase(query)
	query = implicitOR(query)

	stmt := `SELECT ef.name, ef.data_type, ef.description, ef.is_array, ef.pattern
FROM ecs_fields_fts
JOIN ecs_fields ef ON ef.id = ecs_fields_fts.rowid
WHERE ecs_fields_fts MATCH ?
ORDER BY rank
LIMIT ?`

	return t.queryJSON(ctx, toolSearchECSFields, stmt, query, limit)
}

const maxMatchFieldNames = 500

type MatchECSFieldsArgs struct {
	FieldNames []string `json:"field_names" jsonschema:"List of dotted field names to check against ECS (max 500)"`
}

func (t *tools) matchECSFields(ctx context.Context, req *mcp.CallToolRequest, args MatchECSFieldsArgs) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	start := time.Now()
	success := false
	defer func() {
		t.metrics.RecordToolCall(ctx, toolMatchECSFields, success, time.Since(start))
	}()

	if len(args.FieldNames) == 0 {
		return mcpErrorf("field_names must not be empty"), nil, nil
	}
	if len(args.FieldNames) > maxMatchFieldNames {
		return mcpErrorf("field_names exceeds maximum of %d", maxMatchFieldNames), nil, nil
	}

	db := t.db.Load()
	if db == nil {
		t.log.WarnContext(ctx, "Query failed",
			slog.String("tool", toolMatchECSFields),
			slog.String("error", "database not ready"),
			slog.Duration("duration", time.Since(start)))
		t.metrics.RecordError(ctx, "db_not_ready")
		return mcpErrorf("database is still initializing, please retry in a moment"), nil, nil
	}

	// Build parameterized IN clause.
	placeholders := make([]string, len(args.FieldNames))
	queryArgs := make([]any, len(args.FieldNames))
	for i, name := range args.FieldNames {
		placeholders[i] = "?"
		queryArgs[i] = name
	}

	stmt := `SELECT name, data_type, description FROM ecs_fields WHERE name IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := db.QueryContext(ctx, stmt, queryArgs...)
	if err != nil {
		t.log.ErrorContext(ctx, "Query failed",
			slog.String("tool", toolMatchECSFields),
			slog.Any("error", err),
			slog.Duration("duration", time.Since(start)))
		t.metrics.RecordSQLQuery(ctx, false, time.Since(start))
		t.metrics.RecordError(ctx, "sql_query")
		return mcpErrorf("failed to execute query: %v", err), nil, nil
	}
	defer rows.Close()

	// Collect ECS matches into a map.
	type ecsMatch struct {
		DataType    string `json:"ecs_data_type"`
		Description string `json:"ecs_description"`
	}
	matched := make(map[string]ecsMatch)
	for rows.Next() {
		var name, dataType, description string
		if err := rows.Scan(&name, &dataType, &description); err != nil {
			t.metrics.RecordSQLQuery(ctx, false, time.Since(start))
			t.metrics.RecordError(ctx, "sql_scan")
			return mcpErrorf("failed to scan row: %v", err), nil, nil
		}
		matched[name] = ecsMatch{DataType: dataType, Description: description}
	}

	// Build result with all input fields annotated.
	type fieldResult struct {
		Name        string `json:"name"`
		IsECS       bool   `json:"is_ecs"`
		DataType    string `json:"ecs_data_type,omitempty"`
		Description string `json:"ecs_description,omitempty"`
	}
	results := make([]fieldResult, len(args.FieldNames))
	for i, name := range args.FieldNames {
		if m, ok := matched[name]; ok {
			results[i] = fieldResult{Name: name, IsECS: true, DataType: m.DataType, Description: m.Description}
		} else {
			results[i] = fieldResult{Name: name, IsECS: false}
		}
	}

	jsonData, err := json.Marshal(results)
	if err != nil {
		t.metrics.RecordSQLQuery(ctx, false, time.Since(start))
		t.metrics.RecordError(ctx, "json_marshal")
		return mcpErrorf("failed to marshal result: %v", err), nil, nil
	}

	success = true
	t.metrics.RecordSQLQuery(ctx, true, time.Since(start))
	t.log.InfoContext(ctx, "Query executed",
		slog.String("tool", toolMatchECSFields),
		slog.Int("input_count", len(args.FieldNames)),
		slog.Int("match_count", len(matched)),
		slog.Duration("duration", time.Since(start)))
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
	}, nil, nil
}

// splitCamelCase splits camelCase and PascalCase tokens within a query
// into separate words. For example "ProcessTTYAttached" becomes
// "Process TTY Attached" and "sourceIP" becomes "source IP".
// Tokens that are already all-lowercase or all-uppercase are unchanged.
// Underscores are also treated as word boundaries.
func splitCamelCase(query string) string {
	tokens := strings.Fields(query)
	changed := false
	for i, tok := range tokens {
		split := splitCamelCaseToken(tok)
		if split != tok {
			tokens[i] = split
			changed = true
		}
	}
	if !changed {
		return query
	}
	return strings.Join(tokens, " ")
}

func splitCamelCaseToken(s string) string {
	// Replace underscores with spaces first.
	s = strings.ReplaceAll(s, "_", " ")
	if strings.Contains(s, " ") {
		// Recursively process each sub-token from underscore splitting.
		parts := strings.Fields(s)
		for i, p := range parts {
			parts[i] = splitCamelCaseToken(p)
		}
		return strings.Join(parts, " ")
	}

	var buf strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && isUpperRune(r) {
			prev := runes[i-1]
			if isLowerRune(prev) {
				// lowUpp boundary: "sourceIP" → "source IP"
				buf.WriteRune(' ')
			} else if isUpperRune(prev) && i+1 < len(runes) && isLowerRune(runes[i+1]) {
				// UPPUpp+low boundary: "TTYAttached" → "TTY Attached"
				buf.WriteRune(' ')
			}
		}
		buf.WriteRune(r)
	}
	return buf.String()
}

func isUpperRune(r rune) bool { return r >= 'A' && r <= 'Z' }
func isLowerRune(r rune) bool { return r >= 'a' && r <= 'z' }

// fts5Operators are the reserved keywords in FTS5 query syntax.
var fts5Operators = map[string]bool{
	"AND":  true,
	"OR":   true,
	"NOT":  true,
	"NEAR": true,
}

// implicitOR rewrites a plain FTS5 query so space-separated terms use OR
// instead of FTS5's default implicit AND. This makes discovery searches
// additive: fields matching more terms rank higher, but a single matching
// term is enough to return a result.
//
// Queries that already contain FTS5 operators (AND, OR, NOT, NEAR),
// phrase quotes, or prefix wildcards are returned unchanged.
func implicitOR(query string) string {
	// If the query contains FTS5 syntax characters, pass through as-is.
	if strings.ContainsAny(query, `"()*`) {
		return query
	}

	tokens := strings.Fields(query)
	for _, tok := range tokens {
		if fts5Operators[tok] {
			return query
		}
	}

	if len(tokens) <= 1 {
		return query
	}

	return strings.Join(tokens, " OR ")
}

// queryJSON executes a parameterized query, marshals results to JSON, and
// returns them as an MCP text content result. It handles metrics, logging,
// and the "database not ready" case.
func (t *tools) queryJSON(ctx context.Context, toolName, stmt string, queryArgs ...any) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	start := time.Now()
	success := false
	defer func() {
		t.metrics.RecordToolCall(ctx, toolName, success, time.Since(start))
	}()

	db := t.db.Load()
	if db == nil {
		t.log.WarnContext(ctx, "Query failed",
			slog.String("tool", toolName),
			slog.String("error", "database not ready"),
			slog.Duration("duration", time.Since(start)))
		t.metrics.RecordError(ctx, "db_not_ready")
		return mcpErrorf("database is still initializing, please retry in a moment"), nil, nil
	}

	rows, err := db.QueryContext(ctx, stmt, queryArgs...)
	if err != nil {
		t.log.ErrorContext(ctx, "Query failed",
			slog.String("tool", toolName),
			slog.Any("error", err),
			slog.Duration("duration", time.Since(start)))
		t.metrics.RecordSQLQuery(ctx, false, time.Since(start))
		t.metrics.RecordError(ctx, "sql_query")
		return mcpErrorf("failed to execute query: %v", err), nil, nil
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		t.metrics.RecordSQLQuery(ctx, false, time.Since(start))
		t.metrics.RecordError(ctx, "sql_columns")
		return mcpErrorf("failed to get columns: %v", err), nil, nil
	}

	var result []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}

		if err := rows.Scan(pointers...); err != nil {
			t.metrics.RecordSQLQuery(ctx, false, time.Since(start))
			t.metrics.RecordError(ctx, "sql_scan")
			return mcpErrorf("failed to scan row: %v", err), nil, nil
		}

		row := make(map[string]interface{})
		for i, column := range columns {
			val := values[i]
			if val == nil {
				continue // Omit NULL values to reduce token usage.
			}
			if b, ok := val.([]byte); ok {
				row[column] = string(b)
			} else {
				row[column] = val
			}
		}
		result = append(result, row)
	}

	jsonRows, err := json.Marshal(result)
	if err != nil {
		t.metrics.RecordSQLQuery(ctx, false, time.Since(start))
		t.metrics.RecordError(ctx, "json_marshal")
		return mcpErrorf("failed to marshal result: %v", err), nil, nil
	}

	success = true
	t.metrics.RecordSQLQuery(ctx, true, time.Since(start))
	t.log.InfoContext(ctx, "Query executed",
		slog.String("tool", toolName),
		slog.String("statement", stmt),
		slog.Any("args", queryArgs),
		slog.Int("row_count", len(result)),
		slog.Duration("duration", time.Since(start)))
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonRows)},
		},
	}, nil, nil
}

func mcpErrorf(format string, args ...interface{}) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: fmt.Sprintf("ERROR: "+format, args...),
			},
		},
		IsError: true,
	}
}
