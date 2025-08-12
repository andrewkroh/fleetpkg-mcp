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

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type tools struct {
	tables []string
	db     *sql.DB
	log    *slog.Logger
}

func newTools(tables []string, db *sql.DB, log *slog.Logger) *tools {
	return &tools{
		tables: tables,
		db:     db,
		log:    log,
	}
}

func AddTools(s *mcp.Server, tables []string, db *sql.DB, log *slog.Logger) {
	t := newTools(tables, db, log)

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

func (t *tools) getSQLTables(ctx context.Context, ss *mcp.ServerSession, params *mcp.CallToolParamsFor[struct{}]) (*mcp.CallToolResultFor[struct{}], error) {
	schemas := strings.Join(t.tables, "\n")
	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: schemas},
		},
	}, nil
}

type ExecuteQueryArgs struct {
	Statement string `json:"statement" jsonschema:"SQLite query to execute"`
}

func (t *tools) executeQuery(ctx context.Context, ss *mcp.ServerSession, params *mcp.CallToolParamsFor[ExecuteQueryArgs]) (*mcp.CallToolResultFor[struct{}], error) {
	t.log.InfoContext(ctx, "Executing query", "statement", params.Arguments.Statement)

	rows, err := t.db.QueryContext(ctx, params.Arguments.Statement)
	if err != nil {
		t.log.ErrorContext(ctx, "error executing query", "error", err)
		return mcpErrorf[struct{}]("failed to execute query: %v", err), nil
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		t.log.ErrorContext(ctx, "Error getting columns", "error", err)
		return mcpErrorf[struct{}]("failed to get columns: %v", err), nil
	}

	var result []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}

		if err := rows.Scan(pointers...); err != nil {
			t.log.ErrorContext(ctx, "Error scanning row", "error", err)
			return mcpErrorf[struct{}]("failed to scan row: %v", err), nil
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
		t.log.ErrorContext(ctx, "Error marshaling results", slog.Any("error", err))
		return mcpErrorf[struct{}]("failed to marshal result: %v", err), nil
	}

	t.log.InfoContext(ctx, "Query executed successfully", "row_count", len(result))
	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonRows)},
		},
	}, nil
}

func mcpErrorf[T any](format string, args ...interface{}) *mcp.CallToolResultFor[T] {
	return &mcp.CallToolResultFor[T]{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: fmt.Sprintf("ERROR: "+format, args...),
			},
		},
	}
}
