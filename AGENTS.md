# AGENTS.md

This document provides guidance for AI agents (like Claude Code) working on this repository.

## Overview

This project is an MCP (Model Context Protocol) server that provides access to
Fleet integration package data through a SQLite database. The database schema
and queries are managed using [SQLC](https://sqlc.dev/), which generates
type-safe Go code from SQL.

## Database Development Workflow

The project uses SQLC to generate Go code from SQL definitions. This ensures
type-safety and reduces boilerplate when working with the database.

### Key Files

- `internal/database/schema.sql` - Database schema definition (table structures)
- `internal/database/query.sql` - SQL queries that will be called from Go code
- `internal/database/query.sql.go` - Generated Go code (DO NOT EDIT MANUALLY)
- `internal/database/models.go` - Generated Go models (DO NOT EDIT MANUALLY)
- `internal/database/tables.go` - Generated table creation statements (DO NOT EDIT MANUALLY)

### Making Schema Changes

When you need to modify the database structure or add new queries:

1. **Modify the schema**: Edit `internal/database/schema.sql` to add/modify
   tables or columns

2. **Add queries**: If you need to execute queries from Go code, add them to `internal/database/query.sql`
   - Use SQLC annotations like `-- name: QueryName :one` or `-- name: QueryName :exec`
   - Refer to [SQLC documentation](https://docs.sqlc.dev/en/latest/tutorials/getting-started-sqlite.html) for query syntax

3. **Regenerate Go code**: Run the code generation command:
   ```bash
   go -C ./internal/database generate
   ```

   This executes the `//go:generate` directives in `internal/database/doc.go`:
   - Runs `sqlc generate` to create Go code from SQL
   - Runs `gentables` to generate table creation statements
   - Runs `go-licenser` to add license headers

4. **Use the generated code**: Import and use the generated functions from
   `internal/database` in your application code (e.g., in
   `internal/fleetsql/fleetsql.go`)

### Example Workflow

To add support for a new database table and queries:

```bash
# 1. Edit schema.sql to add the table definition
vim internal/database/schema.sql

# 2. Edit query.sql to add insert/select queries
vim internal/database/query.sql

# 3. Regenerate Go code
go -C ./internal/database generate

# 4. Use the generated code in your application
vim internal/fleetsql/fleetsql.go

# 5. Build and test
go build ./...
go test ./...
```

## Testing Changes

After making database changes:

1. Ensure the code compiles: `go build ./...`
2. Run tests: `go test ./...`
3. Test the MCP server manually if needed

## Common Tasks

### Adding a new table

1. Add table definition to `schema.sql`
2. Add insert/query statements to `query.sql`
3. Run `go -C ./internal/database generate`
4. Update application code to use new queries

### Adding a new query for an existing table

1. Add query to `query.sql` with proper SQLC annotation
2. Run `go -C ./internal/database generate`
3. Use the generated function in your code

### Modifying an existing table

1. Update table definition in `schema.sql`
2. Update affected queries in `query.sql`
3. Run `go -C ./internal/database generate`
4. Update application code as needed

## Best Practices

- Always regenerate code after modifying SQL files
- Never manually edit generated Go files
- Use SQLC query annotations for type-safe parameter binding
- Follow existing naming conventions in SQL queries
- Test database changes thoroughly before committing

## Commit instructions

- Use conventional commit style messages.
- Minimize the use of bulleted lists.
- Do not attribute the changes to Claude.
- Follow the 50/72 rule where the title is 50 characters max and body lines are 72 chars max.