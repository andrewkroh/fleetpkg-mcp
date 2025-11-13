// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package fleetsql

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andrewkroh/go-fleetpkg"

	// Register SQLite database driver.
	_ "modernc.org/sqlite"
)

func TestWritePackages(t *testing.T) {
	integrationsDir := os.Getenv("INTEGRATIONS_DIR")
	if integrationsDir == "" {
		t.Skip("INTEGRATIONS_DIR env var is not set.")
	}

	// Read packages from disk.
	start := time.Now()
	pkgs, err := loadPackages(slog.Default(), integrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) == 0 {
		t.Fatal("no packages found")
	}
	t.Logf("Loaded %d packages in %v", len(pkgs), time.Since(start))

	// Open an in-memory database.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Write packages.
	if err = WritePackages(t.Context(), db, pkgs); err != nil {
		t.Fatal(err)
	}

	// Perform a simple smoke test.
	r, err := db.ExecContext(t.Context(), `SELECT count(*) FROM integrations WHERE name = 'elasticsearch'`)
	if err != nil {
		t.Fatal(err)
	}
	count, err := r.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("elasticsearch integration not found")
	}
}

func loadPackages(log *slog.Logger, integrationsDir string) ([]fleetpkg.Integration, error) {
	// Load packages from disk.
	packages, err := filepath.Glob(filepath.Join(integrationsDir, "packages/*"))
	if err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages found in %s", integrationsDir)
	}

	var integrations []fleetpkg.Integration
	for _, pkgPath := range packages {
		p, err := fleetpkg.Read(pkgPath)
		if err != nil {
			return nil, err
		}
		integrations = append(integrations, *p)
	}
	log.Info("Discovered packages", slog.Int("count", len(integrations)))

	return integrations, nil
}
