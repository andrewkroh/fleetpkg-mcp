// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andrewkroh/go-fleetpkg"
	"github.com/gorilla/handlers"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/andrewkroh/fleetpkg-mcp/internal/fleetsql"
	fleetmcp "github.com/andrewkroh/fleetpkg-mcp/internal/mcp"

	// Register SQLite database driver.
	_ "modernc.org/sqlite"
)

var (
	httpAddr        = flag.String("http", "", "listen for HTTP at this address, instead of stdin/stdout")
	noLog           = flag.Bool("no-log", false, "if set, disables logging")
	logLevel        = flag.String("log-level", "info", "log level (debug, info, warn, error)")
	integrationsDir = flag.String("dir", "", "path to elastic/integrations directory")
	refresh         = flag.String("refresh", "", "periodically refresh database at this interval (e.g., 1h, 30m)")
	gitPull         = flag.Bool("git-pull", false, "run 'git pull' on the integrations directory before each database load")
	version         = flag.Bool("version", false, "print version and exit")
)

func main() {
	flag.Parse()

	if *version {
		fmt.Println(buildVersion())
		return
	}

	if *integrationsDir == "" {
		fmt.Fprintln(os.Stderr, "ERROR: -dir flag is required")
		fmt.Fprintln(os.Stderr, "Example: -dir /data/integrations")
		os.Exit(2)
	}

	// Warn if -git-pull not set and dir doesn't exist
	if !*gitPull {
		if _, err := os.Stat(*integrationsDir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "ERROR: directory %q does not exist and -git-pull is not enabled\n", *integrationsDir)
			fmt.Fprintln(os.Stderr, "Either create the directory with integrations data, or use -git-pull to clone automatically")
			os.Exit(2)
		}
	}

	if err := run(*integrationsDir); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run(integrationsDir string) error {
	// Set up logging.
	var logOutput io.Writer = os.Stderr
	if *noLog {
		logOutput = io.Discard
	}

	log, err := logger(logOutput)
	if err != nil {
		return err
	}
	slog.SetDefault(log)

	modVer, vcsRef := buildVersion()
	log.Info("fleetpkg-mcp is starting...", slog.String("version", modVer), slog.String("vcs_ref", vcsRef))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Create atomic DB pointer for lazy initialization
	dbPtr := &atomic.Pointer[sql.DB]{}

	// Create MCP server immediately
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "fleetpkg",
		Title:   "Elastic Fleet Integration Package metadata MCP server",
		Version: modVer + " (" + vcsRef + ")",
	}, nil)
	fleetmcp.AddTools(s, fleetsql.TableSchemas(), dbPtr, log)

	// Track database file path for cleanup.
	var dbPath string
	var dbPathMu sync.Mutex

	// Parse refresh interval from flag or environment variable.
	var refreshInterval time.Duration
	refreshStr := *refresh
	if refreshStr == "" {
		// Fall back to FLEETPKG_MCP_REFRESH_INTERVAL environment variable
		refreshStr = os.Getenv("FLEETPKG_MCP_REFRESH_INTERVAL")
	}
	if refreshStr != "" {
		var err error
		refreshInterval, err = time.ParseDuration(refreshStr)
		if err != nil {
			return fmt.Errorf("invalid refresh duration %q: %w", refreshStr, err)
		}
		if refreshInterval <= 0 {
			return fmt.Errorf("refresh interval must be positive, got %v", refreshInterval)
		}
		source := "flag"
		if *refresh == "" {
			source = "FLEETPKG_MCP_REFRESH_INTERVAL env"
		}
		log.Info("Periodic refresh enabled", slog.Duration("interval", refreshInterval), slog.String("source", source))
	}

	// Channel to signal initialization completion (for refresh goroutine).
	initDoneCh := make(chan struct{})

	// Start initialization in background
	initErrCh := make(chan error, 1)
	go func() {
		start := time.Now()
		if *gitPull {
			if err := ensureGitRepo(ctx, log, integrationsDir); err != nil {
				log.Error("Failed to ensure git repository", slog.Any("error", err))
				initErrCh <- fmt.Errorf("failed to ensure git repository: %w", err)
				return
			}
		}
		log.Info("Starting database initialization...")
		db, path, err := initializeDatabase(ctx, log, integrationsDir)
		if err != nil {
			log.Error("Database initialization failed", slog.Any("error", err))
			initErrCh <- err
			return
		}
		dbPtr.Store(db)
		dbPathMu.Lock()
		dbPath = path
		dbPathMu.Unlock()
		log.Info("Database initialization completed", slog.Duration("duration", time.Since(start)), slog.String("path", path))
		close(initDoneCh)
		close(initErrCh)
	}()

	// Start periodic refresh goroutine if enabled.
	if refreshInterval > 0 {
		go func() {
			// Wait for initial initialization to complete successfully.
			select {
			case <-ctx.Done():
				return
			case <-initDoneCh:
				// Initialization succeeded, proceed with periodic refresh.
			}

			// Get the database path.
			dbPathMu.Lock()
			path := dbPath
			dbPathMu.Unlock()

			ticker := time.NewTicker(refreshInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if *gitPull {
						if err := ensureGitRepo(ctx, log, integrationsDir); err != nil {
							log.Error("Failed to update git repository before refresh", slog.Any("error", err))
						}
					}
					log.Info("Starting periodic database refresh...")
					start := time.Now()
					newDB, err := refreshDatabase(ctx, log, path, integrationsDir)
					if err != nil {
						log.Error("Periodic database refresh failed", slog.Any("error", err))
						continue
					}

					// Atomically swap the old database with the new one.
					oldDB := dbPtr.Swap(newDB)
					if oldDB != nil {
						oldDB.Close()
					}
					log.Info("Periodic database refresh completed", slog.Duration("duration", time.Since(start)))
				}
			}
		}()
	}

	// Set up cleanup on exit.
	defer func() {
		// Close database connection.
		if db := dbPtr.Load(); db != nil {
			if err := db.Close(); err != nil {
				log.Error("Failed to close database", slog.Any("error", err))
			}
		}

		// Delete database file.
		dbPathMu.Lock()
		path := dbPath
		dbPathMu.Unlock()

		if path != "" {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				log.Error("Failed to remove database file", slog.String("path", path), slog.Any("error", err))
			} else if err == nil {
				log.Info("Cleaned up database file", slog.String("path", path))
			}
		}
	}()

	// Listen over HTTP.
	if *httpAddr != "" {
		var handler http.Handler = mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return s }, nil)

		listener, err := net.Listen("tcp", *httpAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on %q: %w", *httpAddr, err)
		}
		go func() {
			<-ctx.Done()
			listener.Close()
		}()

		log.Info("fleetpkg-mcp handler listening",
			slog.String("addr", "http://"+listener.Addr().String()))

		if !*noLog {
			handler = handlers.CombinedLoggingHandler(os.Stdout, handler)
		}

		// Serve HTTP in goroutine
		serveDone := make(chan error, 1)
		go func() {
			serveDone <- http.Serve(listener, handler)
		}()

		// Wait for context cancellation, init error, or serve error
		select {
		case <-ctx.Done():
			return nil
		case err := <-initErrCh:
			if err != nil {
				return fmt.Errorf("initialization failed: %w", err)
			}
			// Init succeeded, wait for serve to complete
			return <-serveDone
		case err := <-serveDone:
			return fmt.Errorf("failed to serve http: %w", err)
		}
	}

	// Stdin/stdout comms - also start immediately
	serveDone := make(chan error, 1)
	go func() {
		t := &mcp.LoggingTransport{
			Transport: &mcp.StdioTransport{},
			Writer:    logOutput,
		}
		serveDone <- s.Run(ctx, t)
	}()

	// Wait for context cancellation, init error, or serve error
	select {
	case <-ctx.Done():
		return nil
	case err := <-initErrCh:
		if err != nil {
			return fmt.Errorf("initialization failed: %w", err)
		}
		// Init succeeded, wait for serve to complete
		return <-serveDone
	case err := <-serveDone:
		if err != nil {
			return fmt.Errorf("failed to run stdio server: %w", err)
		}
		return nil
	}
}

func logger(sink io.Writer) (*slog.Logger, error) {
	level := new(slog.LevelVar)
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		return nil, err
	}

	return slog.New(
		slog.NewTextHandler(
			sink,
			&slog.HandlerOptions{
				Level: level,
			},
		)), nil
}

func buildVersion() (modVersion, vcsRef string) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", ""
	}

	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			vcsRef = setting.Value
			break
		}
	}

	return info.Main.Version, vcsRef
}

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

// initializeDatabase loads packages and creates a read-only SQLite database.
// Returns the database connection and the database file path.
func initializeDatabase(ctx context.Context, log *slog.Logger, integrationsDir string) (*sql.DB, string, error) {
	// Get the per-process database path.
	dbPath, err := getDatabasePath()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get database path: %w", err)
	}

	// Read packages from the integrations repo.
	pkgs, err := loadPackages(log, integrationsDir)
	if err != nil {
		return nil, dbPath, fmt.Errorf("failed to load packages: %w", err)
	}

	// Create a new DB (each process has its own file, so no need to remove).
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		return nil, dbPath, fmt.Errorf("failed to open new database: %w", err)
	}

	if err = fleetsql.WritePackages(ctx, db, pkgs); err != nil {
		db.Close()
		return nil, dbPath, fmt.Errorf("failed to write packages to DB: %w", err)
	}
	if err = db.Close(); err != nil {
		return nil, dbPath, fmt.Errorf("failed to close database: %w", err)
	}

	// Open the database as read-only.
	db, err = sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, dbPath, fmt.Errorf("failed to open database readonly: %w", err)
	}

	return db, dbPath, nil
}

// refreshDatabase rebuilds the database with the latest package data.
// It returns a new read-only database connection.
// Uses a temporary file and atomic rename to ensure the database file always exists.
func refreshDatabase(ctx context.Context, log *slog.Logger, dbPath, integrationsDir string) (*sql.DB, error) {
	// Read packages from the integrations repo.
	pkgs, err := loadPackages(log, integrationsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	// Write to a temporary file first.
	tmpPath := dbPath + ".tmp"
	if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove existing temp database: %w", err)
	}

	db, err := sql.Open("sqlite", "file:"+tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open temp database: %w", err)
	}

	if err = fleetsql.WritePackages(ctx, db, pkgs); err != nil {
		db.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to write packages to DB: %w", err)
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

	// Open the database as read-only.
	db, err = sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("failed to open database readonly: %w", err)
	}

	return db, nil
}

// gitPullRepo runs 'git pull --ff-only' in the given directory.
// Environment variables are set to prevent any interactive prompts that
// would block in an automated/containerized environment.
func gitPullRepo(ctx context.Context, log *slog.Logger, dir string) error {
	log.Info("Running git pull...", slog.String("dir", dir))
	start := time.Now()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "pull", "--ff-only", "--no-color")
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",                // Disable HTTPS credential prompts.
		"GIT_SSH_COMMAND=ssh -o BatchMode=yes", // Disable SSH passphrase/password prompts.
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull failed: %w: %s", err, output)
	}
	log.Info("Git pull completed", slog.Duration("duration", time.Since(start)), slog.String("output", string(output)))
	return nil
}

// gitCloneRepo clones the elastic/integrations repository into the given directory.
// It uses a shallow clone to minimize download size and time.
func gitCloneRepo(ctx context.Context, log *slog.Logger, dir string) error {
	log.Info("Cloning integrations repository...", slog.String("dir", dir))
	start := time.Now()

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "clone",
		"--depth=1",
		"--single-branch",
		"--no-tags",
		"https://github.com/elastic/integrations.git",
		dir)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -o BatchMode=yes",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w: %s", err, output)
	}

	log.Info("Git clone completed", slog.Duration("duration", time.Since(start)))
	return nil
}

// ensureGitRepo ensures the integrations repository exists and is up to date.
// It clones the repository if it doesn't exist or is empty, and pulls updates
// if it already exists.
func ensureGitRepo(ctx context.Context, log *slog.Logger, dir string) error {
	// Check if directory exists
	stat, err := os.Stat(dir)
	if os.IsNotExist(err) {
		// Directory doesn't exist, clone it
		return gitCloneRepo(ctx, log, dir)
	}
	if err != nil {
		return fmt.Errorf("failed to check directory: %w", err)
	}

	if !stat.IsDir() {
		return fmt.Errorf("%q is not a directory", dir)
	}

	// Check if directory is empty
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	if len(entries) == 0 {
		// Empty directory, clone into it
		return gitCloneRepo(ctx, log, dir)
	}

	// Check if it's a git repository
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Errorf("directory %q exists but is not a git repository; please remove it or use a different path", dir)
	}

	// Existing git repo, pull updates
	return gitPullRepo(ctx, log, dir)
}

// loadPackages loads integration packages from the specified directory.
// It returns a slice of Integration structs or an error if loading fails.
func loadPackages(log *slog.Logger, integrationsDir string) ([]fleetpkg.Integration, error) {
	packages, err := filepath.Glob(filepath.Join(integrationsDir, "packages/*"))
	if err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages found in %s", integrationsDir)
	}

	// Use bounded concurrency based on GOMAXPROCS
	workers := runtime.GOMAXPROCS(0)
	type result struct {
		integration fleetpkg.Integration
		err         error
	}

	jobs := make(chan string, len(packages))
	results := make(chan result, len(packages))

	// Start worker pool
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pkgPath := range jobs {
				p, err := fleetpkg.Read(pkgPath, fleetpkg.WithChangelogDates())
				if err != nil {
					results <- result{err: err}
					return
				}
				results <- result{integration: *p}
			}
		}()
	}

	// Send jobs
	for _, pkgPath := range packages {
		jobs <- pkgPath
	}
	close(jobs)

	// Wait for all workers to finish
	wg.Wait()
	close(results)

	// Collect results
	integrations := make([]fleetpkg.Integration, 0, len(packages))
	for res := range results {
		if res.err != nil {
			return nil, res.err
		}
		integrations = append(integrations, res.integration)
	}

	log.Info("Discovered packages", slog.Int("count", len(integrations)))

	return integrations, nil
}
