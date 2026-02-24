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
}

const (
	toolGetSQLTables     = "fleetpkg_get_sql_tables"
	toolExecuteSQLQuery  = "fleetpkg_execute_sql_query"
	toolSearchDocs       = "fleetpkg_search_docs"
	toolSearchChangelogs = "fleetpkg_search_changelogs"
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
