// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/andrewkroh/go-ecs"
	"github.com/andrewkroh/go-package-spec/pkgreader"
	"github.com/andrewkroh/go-package-spec/pkgspec"
	"github.com/andrewkroh/go-package-spec/pkgsql"
)

// getDatabasePath returns the path to the database file in an OS-specific
// application data directory. Each process gets its own unique file using the PID.
func getDatabasePath() (string, error) {
	var dbDir string
	var err error

	switch runtime.GOOS {
	case "windows":
		// Windows: Use LOCALAPPDATA
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			dbDir = filepath.Join(localAppData, "fleetpkg-mcp")
		} else {
			// Fallback to UserConfigDir if LOCALAPPDATA is not set
			dbDir, err = os.UserConfigDir()
			if err != nil {
				return "", fmt.Errorf("failed to get user config directory: %w", err)
			}
			dbDir = filepath.Join(dbDir, "fleetpkg-mcp")
		}
	default:
		// Linux/macOS: Use home directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		dbDir = filepath.Join(homeDir, ".fleetpkg-mcp")
	}

	// Create the directory if it doesn't exist.
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create database directory: %w", err)
	}

	// Create unique filename using PID.
	pid := os.Getpid()
	dbName := fmt.Sprintf("fleetpkg-%d.sqlite", pid)
	return filepath.Join(dbDir, dbName), nil
}

// InitializeDatabase loads packages and creates a read-only SQLite database.
// Returns the database connection and the database file path.
func InitializeDatabase(ctx context.Context, log *slog.Logger, integrationsDir string) (*sql.DB, string, error) {
	// Get the per-process database path.
	dbPath, err := getDatabasePath()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get database path: %w", err)
	}

	// Create a new DB (each process has its own file, so no need to remove).
	// SQLite only supports a single writer, so limit to one connection.
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		return nil, dbPath, fmt.Errorf("failed to open new database: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err = BuildDatabase(ctx, log, db, integrationsDir); err != nil {
		db.Close()
		return nil, dbPath, fmt.Errorf("failed to build database: %w", err)
	}
	if err = db.Close(); err != nil {
		return nil, dbPath, fmt.Errorf("failed to close database: %w", err)
	}

	// Open the database as read-only. Limit the pool size so that
	// locked OS threads are released promptly when this DB is closed
	// during a refresh swap.
	db, err = sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, dbPath, fmt.Errorf("failed to open database readonly: %w", err)
	}
	db.SetMaxOpenConns(4)

	return db, dbPath, nil
}

// RefreshDatabase rebuilds the database with the latest package data.
// It returns a new read-only database connection.
// Uses a temporary file and atomic rename to ensure the database file always exists.
func RefreshDatabase(ctx context.Context, log *slog.Logger, dbPath, integrationsDir string) (*sql.DB, error) {
	// Write to a temporary file first.
	tmpPath := dbPath + ".tmp"
	if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove existing temp database: %w", err)
	}

	// SQLite only supports a single writer, so limit to one connection.
	db, err := sql.Open("sqlite", "file:"+tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open temp database: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err = BuildDatabase(ctx, log, db, integrationsDir); err != nil {
		db.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to build database: %w", err)
	}
	if err = db.Close(); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to close temp database: %w", err)
	}

	// Atomically replace the old database with the new one.
	if err := os.Rename(tmpPath, dbPath); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to replace database: %w", err)
	}

	// Open the database as read-only. Limit the pool size so that
	// locked OS threads are released promptly when this DB is closed
	// during a refresh swap.
	db, err = sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("failed to open database readonly: %w", err)
	}
	db.SetMaxOpenConns(4)

	return db, nil
}

// BuildDatabase creates tables, reads packages via a bounded worker pool,
// writes each to the DB as it arrives, and rebuilds FTS indexes at the end.
// The bounded results channel keeps memory usage low by only buffering a
// limited number of packages at a time.
func BuildDatabase(ctx context.Context, log *slog.Logger, db *sql.DB, integrationsDir string) error {
	packagesDir := filepath.Join(integrationsDir, "packages")
	pkgPaths, err := pkgreader.ListPackages(packagesDir)
	if err != nil {
		return fmt.Errorf("listing packages: %w", err)
	}

	// Create tables.
	for _, ddl := range append(pkgsql.TableSchemas(), ECSTableSchemas()...) {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("creating tables: %w", err)
		}
	}

	// Reader options shared by all packages.
	baseOpts := []pkgreader.Option{
		pkgreader.WithGitMetadata(),
		pkgreader.WithImageMetadata(),
		pkgreader.WithTestConfigs(),
		pkgreader.WithAgentTemplates(),
		pkgreader.WithCodeowners(filepath.Join(integrationsDir, ".github", "CODEOWNERS")),
	}

	// Use more workers than CPUs since package reading is I/O bound
	// (git blame subprocess, file reads).
	workers := 4 * runtime.NumCPU()

	type result struct {
		pkg  *pkgreader.Package
		name string
		err  error
	}

	work := make(chan string, workers)
	results := make(chan result, workers)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pkgPath := range work {
				rel, err := filepath.Rel(integrationsDir, pkgPath)
				if err != nil {
					results <- result{name: pkgPath, err: err}
					continue
				}
				prefix := filepath.ToSlash(rel)
				opts := append(baseOpts, pkgreader.WithPathPrefix(prefix))
				pkg, err := pkgreader.Read(pkgPath, opts...)
				results <- result{pkg: pkg, name: prefix, err: err}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	go func() {
		for _, p := range pkgPaths {
			work <- p
		}
		close(work)
	}()

	// Consume results and write to DB one at a time.
	var loaded int
	var firstErr error
	for r := range results {
		if firstErr != nil {
			// Drain remaining results so worker and waiter goroutines can exit.
			continue
		}
		if r.err != nil {
			firstErr = fmt.Errorf("reading package %s: %w", r.name, r.err)
			continue
		}

		writeOpts := []pkgsql.Option{pkgsql.WithDocContent(pkgsql.OSDocReader)}
		if opt := ecsLookupForPackage(r.pkg); opt != nil {
			writeOpts = append(writeOpts, opt)
		}
		if err := pkgsql.WritePackage(ctx, db, r.pkg, writeOpts...); err != nil {
			firstErr = fmt.Errorf("writing package %s: %w", r.name, err)
			continue
		}
		loaded++
	}
	if firstErr != nil {
		return firstErr
	}

	log.Info("Loaded packages", slog.Int("count", loaded))

	// Load ECS fields into the ecs_fields table.
	if err := loadECSFields(ctx, db); err != nil {
		return fmt.Errorf("loading ECS fields: %w", err)
	}

	// Rebuild FTS indexes after all writes.
	return pkgsql.RebuildFTS(ctx, db)
}

const ecsFieldsTableSchema = `CREATE TABLE IF NOT EXISTS ecs_fields (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  data_type TEXT NOT NULL,
  description TEXT NOT NULL,
  is_array BOOLEAN NOT NULL DEFAULT FALSE,
  pattern TEXT,
  search_text TEXT NOT NULL
);`

const ecsFieldsFTSSchema = `CREATE VIRTUAL TABLE IF NOT EXISTS ecs_fields_fts USING fts5(
  search_text,
  content=ecs_fields,
  content_rowid=id,
  tokenize='porter unicode61'
);`

// ECSTableSchemas returns the DDL statements for the ECS fields tables.
func ECSTableSchemas() []string {
	return []string{ecsFieldsTableSchema, ecsFieldsFTSSchema}
}

// loadECSFields loads all fields from the latest ECS version into the
// ecs_fields table and rebuilds the FTS index.
func loadECSFields(ctx context.Context, db *sql.DB) error {
	fields, err := ecs.Fields("")
	if err != nil {
		return fmt.Errorf("loading ECS fields: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO ecs_fields (name, data_type, description, is_array, pattern, search_text)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing insert: %w", err)
	}
	defer stmt.Close()

	for _, f := range fields {
		searchText := strings.ReplaceAll(f.Name, ".", " ") + " " + f.Description
		var pattern *string
		if f.Pattern != "" {
			pattern = &f.Pattern
		}
		if _, err := stmt.ExecContext(ctx, f.Name, f.DataType, f.Description, f.Array, pattern, searchText); err != nil {
			return fmt.Errorf("inserting ECS field %s: %w", f.Name, err)
		}
	}

	// Rebuild the ECS fields FTS index.
	if _, err := tx.ExecContext(ctx, `INSERT INTO ecs_fields_fts(ecs_fields_fts) VALUES('rebuild')`); err != nil {
		return fmt.Errorf("rebuilding ECS fields FTS: %w", err)
	}

	return tx.Commit()
}

// ecsLookupForPackage returns a pkgsql.Option that resolves ECS field
// references for a package, or nil if the package has no ECS dependency.
func ecsLookupForPackage(pkg *pkgreader.Package) pkgsql.Option {
	if pkg.Build == nil || pkg.Build.Dependencies.ECS.Reference == "" {
		return nil
	}
	ref := strings.TrimPrefix(pkg.Build.Dependencies.ECS.Reference, "git@")
	return pkgsql.WithECSLookup(func(name string) *pkgspec.ECSFieldDefinition {
		field, _ := ecs.Lookup(name, ref)
		if field == nil {
			return nil
		}
		return &pkgspec.ECSFieldDefinition{
			DataType:    field.DataType,
			Description: field.Description,
			Pattern:     field.Pattern,
			Array:       field.Array,
		}
	})
}
