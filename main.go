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
	"os/signal"
	"path/filepath"
	"runtime/debug"
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
		os.Exit(2)
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
	log.Info("fleetpkg-mcp is starting...", slog.Any("version", modVer), slog.Any("vcs_ref", vcsRef))

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

	// Start initialization in background
	initErrCh := make(chan error, 1)
	go func() {
		start := time.Now()
		log.Info("Starting database initialization...")
		db, err := initializeDatabase(ctx, log, integrationsDir)
		if err != nil {
			log.Error("Database initialization failed", "error", err)
			initErrCh <- err
			return
		}
		dbPtr.Store(db)
		log.Info("Database initialization completed", slog.Any("duration", time.Since(start)))
		close(initErrCh)
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

// initializeDatabase loads packages and creates a read-only SQLite database.
func initializeDatabase(ctx context.Context, log *slog.Logger, integrationsDir string) (*sql.DB, error) {
	// Read packages from the integrations repo.
	pkgs, err := loadPackages(log, integrationsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	// Create a new DB.
	if err = os.Remove("fleetpkg.db"); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove existing database: %w", err)
	}
	db, err := sql.Open("sqlite", "file:fleetpkg.db")
	if err != nil {
		return nil, fmt.Errorf("failed to open new database: %w", err)
	}

	if err = fleetsql.WritePackages(ctx, db, pkgs); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to write packages to DB: %w", err)
	}
	if err = db.Close(); err != nil {
		return nil, fmt.Errorf("failed to close database: %w", err)
	}

	// Open the database as read-only.
	db, err = sql.Open("sqlite", "file:fleetpkg.db?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("failed to open database readonly: %w", err)
	}

	return db, nil
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

	var integrations []fleetpkg.Integration
	for _, pkgPath := range packages {
		p, err := fleetpkg.Read(pkgPath)
		if err != nil {
			return nil, err
		}
		integrations = append(integrations, *p)
	}
	log.Info("Discovered packages", "count", len(integrations))

	return integrations, nil
}
