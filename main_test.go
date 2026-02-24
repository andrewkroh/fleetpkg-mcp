// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"

	_ "modernc.org/sqlite"
)

// threadCount returns the number of OS threads created by the Go runtime.
// This uses the runtime/pprof threadcreate profile which tracks cumulative
// thread creations. modernc.org/sqlite calls runtime.LockOSThread() for
// each connection, so unbounded connection pools leak OS threads.
func threadCount() int {
	return pprof.Lookup("threadcreate").Count()
}

// TestBuildDatabaseGoroutineLeak runs buildDatabase repeatedly and checks
// that goroutines don't grow, catching leaks from workers, producers, or
// waiters that fail to exit.
func TestBuildDatabaseGoroutineLeak(t *testing.T) {
	dir := createTestIntegrations(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	// Warm up: run once to stabilize runtime goroutines (GC, finalizers, etc).
	db := openTestDB(t)
	if err := buildDatabase(ctx, log, db, dir); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Force GC to finalize any lingering state.
	runtime.GC()
	runtime.Gosched()

	baseline := runtime.NumGoroutine()
	t.Logf("baseline goroutines after warmup: %d", baseline)

	const iterations = 10

	for i := range iterations {
		db := openTestDB(t)
		if err := buildDatabase(ctx, log, db, dir); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		db.Close()
	}

	// Force GC and let goroutines settle.
	runtime.GC()
	runtime.Gosched()

	final := runtime.NumGoroutine()
	t.Logf("goroutines after %d iterations: %d (baseline: %d)", iterations, final, baseline)

	// Allow a small margin (2) for runtime jitter, but not proportional growth.
	if final > baseline+2 {
		t.Errorf("goroutine leak detected: baseline=%d, final=%d (leaked %d)", baseline, final, final-baseline)
	}
}

// TestBuildDatabaseGoroutineLeak_WithErrors verifies that goroutines don't
// leak even when buildDatabase encounters read errors (e.g. malformed packages).
func TestBuildDatabaseGoroutineLeak_WithErrors(t *testing.T) {
	dir := createTestIntegrationsWithBadPkg(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	// Warm up.
	db := openTestDB(t)
	_ = buildDatabase(ctx, log, db, dir)
	db.Close()

	runtime.GC()
	runtime.Gosched()

	baseline := runtime.NumGoroutine()
	t.Logf("baseline goroutines after warmup: %d", baseline)

	const iterations = 10

	for range iterations {
		db := openTestDB(t)
		_ = buildDatabase(ctx, log, db, dir) // Errors expected.
		db.Close()
	}

	runtime.GC()
	runtime.Gosched()

	final := runtime.NumGoroutine()
	t.Logf("goroutines after %d error iterations: %d (baseline: %d)", iterations, final, baseline)

	if final > baseline+2 {
		t.Errorf("goroutine leak on error path: baseline=%d, final=%d (leaked %d)", baseline, final, final-baseline)
	}
}

// TestRefreshDatabaseThreadLeak simulates the full init+refresh cycle and
// checks that OS threads don't grow. modernc.org/sqlite calls
// runtime.LockOSThread() per connection, so uncapped connection pools cause
// thread accumulation visible only via the threadcreate pprof profile —
// not via runtime.NumGoroutine().
func TestRefreshDatabaseThreadLeak(t *testing.T) {
	dir := createTestIntegrations(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	// Initialize.
	db, dbPath, err := initializeDatabase(ctx, log, dir)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Warm up with one refresh to stabilize thread count.
	db, err = refreshDatabase(ctx, log, dbPath, dir)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	runtime.GC()
	runtime.Gosched()

	baselineThreads := threadCount()
	baselineGoroutines := runtime.NumGoroutine()
	t.Logf("baseline after warmup: threads=%d, goroutines=%d", baselineThreads, baselineGoroutines)

	const iterations = 10

	for i := range iterations {
		db, err = refreshDatabase(ctx, log, dbPath, dir)
		if err != nil {
			t.Fatalf("refresh iteration %d: %v", i, err)
		}
		db.Close()
	}

	runtime.GC()
	runtime.Gosched()

	finalThreads := threadCount()
	finalGoroutines := runtime.NumGoroutine()
	t.Logf("after %d refresh iterations: threads=%d (baseline: %d), goroutines=%d (baseline: %d)",
		iterations, finalThreads, baselineThreads, finalGoroutines, baselineGoroutines)

	// Thread count: allow a small margin (3) for runtime jitter, but
	// not proportional growth. Without SetMaxOpenConns, each refresh
	// cycle creates new connections that pin new OS threads.
	if finalThreads > baselineThreads+3 {
		t.Errorf("OS thread leak detected: baseline=%d, final=%d (leaked %d)",
			baselineThreads, finalThreads, finalThreads-baselineThreads)
	}

	if finalGoroutines > baselineGoroutines+2 {
		t.Errorf("goroutine leak in refresh cycle: baseline=%d, final=%d (leaked %d)",
			baselineGoroutines, finalGoroutines, finalGoroutines-baselineGoroutines)
	}
}

// openTestDB opens a temporary SQLite database with SetMaxOpenConns(1),
// matching the production write-path configuration.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.sqlite")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	return db
}

// createTestIntegrations creates a minimal integrations directory with 3 small
// packages. The directory structure mirrors elastic/integrations. A git repo
// is initialized so that WithGitMetadata() succeeds.
func createTestIntegrations(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for i := range 3 {
		createMinimalPackage(t, dir, i)
	}
	initGitRepo(t, dir)
	return dir
}

// createTestIntegrationsWithBadPkg creates an integrations directory containing
// both valid and invalid packages, so buildDatabase will encounter errors.
func createTestIntegrationsWithBadPkg(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// One valid package.
	createMinimalPackage(t, dir, 0)

	// One bad package: has a directory but no manifest.yml.
	badPkg := filepath.Join(dir, "packages", "bad_package")
	if err := os.MkdirAll(badPkg, 0o755); err != nil {
		t.Fatal(err)
	}

	initGitRepo(t, dir)
	return dir
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "test"},
		{"add", "."},
		{"commit", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

func createMinimalPackage(t *testing.T, dir string, index int) {
	t.Helper()

	names := []string{"alpha", "beta", "gamma"}
	name := names[index%len(names)]
	pkgDir := filepath.Join(dir, "packages", name)

	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := `format_version: "3.3.0"
name: ` + name + `
title: Test ` + name + `
version: "1.0.0"
type: integration
description: A test package.
categories:
  - security
owner:
  github: elastic/test-team
  type: elastic
policy_templates:
  - name: default
    title: Default
    description: Collect data.
    inputs:
      - type: logfile
        title: Logs
        description: Collect logs.
`
	if err := os.WriteFile(filepath.Join(pkgDir, "manifest.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	changelog := `- version: "1.0.0"
  changes:
    - description: Initial release.
      type: enhancement
      link: https://github.com/test/test/pull/1
`
	if err := os.WriteFile(filepath.Join(pkgDir, "changelog.yml"), []byte(changelog), 0o644); err != nil {
		t.Fatal(err)
	}
}
