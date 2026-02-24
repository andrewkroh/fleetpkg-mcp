# AGENTS.md

This document provides guidance for AI agents (like Claude Code) working on this repository.

## Overview

This project is an MCP (Model Context Protocol) server that provides access to
Fleet integration package data through a SQLite database. The database schema,
queries, and package reading logic are provided by the
[go-package-spec](https://github.com/andrewkroh/go-package-spec) dependency
(`pkgsql`, `pkgreader`, `pkgspec` packages).

## Project Structure

- `main.go` - Entry point: flag parsing, validation, then calls `app.Run()`
- `internal/app/` - Application orchestration: config, server lifecycle,
  database init/refresh, git operations, HTTP middleware
- `internal/mcp/` - MCP tool definitions and handlers
- `internal/otelsetup/` - OpenTelemetry metrics setup
- `internal/slogutil/` - Structured logging utilities

## Testing Changes

1. Ensure the code compiles: `go build ./...`
2. Run static analysis: `go vet ./...`
3. Run tests: `go test ./...`
4. Run formatter: `gofumpt -w -extra .' from (`go install mvdan.cc/gofumpt@latest`)
4. Test the MCP server manually if needed

## Commit instructions

- Use conventional commit style messages.
- Minimize the use of bulleted lists.
- Follow the 50/72 rule where the title is 50 characters max and body lines are 72 chars max.

