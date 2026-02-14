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
}

const (
	toolGetSQLTables    = "fleetpkg_get_sql_tables"
	toolExecuteSQLQuery = "fleetpkg_execute_sql_query"
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

func (t *tools) executeQuery(ctx context.Context, req *mcp.CallToolRequest, args ExecuteQueryArgs) (*mcp.CallToolResult, any, error) {
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
