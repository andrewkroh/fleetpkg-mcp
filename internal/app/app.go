// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package app

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andrewkroh/go-package-spec/pkgsql"
	"github.com/gorilla/handlers"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	fleetmcp "github.com/andrewkroh/fleetpkg-mcp/internal/mcp"
	"github.com/andrewkroh/fleetpkg-mcp/internal/otelsetup"
	"github.com/andrewkroh/fleetpkg-mcp/internal/slogutil"

	// Register SQLite database driver.
	_ "modernc.org/sqlite"
)

// Config holds all configuration parsed from command-line flags.
type Config struct {
	IntegrationsDir string // -dir flag
	HTTPAddr        string // -http flag
	PprofAddr       string // -pprof flag (env fallback in Run)
	NoLog           bool   // -no-log flag
	LogLevel        string // -log-level flag (default "info")
	Refresh         string // -refresh flag (env fallback in Run)
	GitPull         bool   // -git-pull flag
}

// BuildVersion returns the module version and VCS revision from build info.
func BuildVersion() (modVersion, vcsRef string) {
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

// Run is the main entry point for the application. It sets up logging,
// metrics, the MCP server, and manages the database lifecycle.
func Run(cfg Config) error {
	// Set up logging.
	var logOutput io.Writer = os.Stderr
	if cfg.NoLog {
		logOutput = io.Discard
	}

	log, err := newLogger(cfg.LogLevel, logOutput)
	if err != nil {
		return err
	}
	slog.SetDefault(log)

	modVer, vcsRef := BuildVersion()
	log.Info("fleetpkg-mcp is starting...", slog.String("version", modVer), slog.String("vcs_ref", vcsRef))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Set up OpenTelemetry metrics.
	shutdownMetrics, err := otelsetup.SetupMetrics(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup metrics: %w", err)
	}
	defer func() {
		if err := shutdownMetrics(ctx); err != nil {
			log.Error("Failed to shutdown metrics", slog.Any("error", err))
		}
	}()

	// Initialize metric instruments.
	metrics, err := otelsetup.NewMetrics()
	if err != nil {
		return fmt.Errorf("failed to create metrics: %w", err)
	}

	// Start pprof debug server if requested (flag or env var).
	pprofAddr := cfg.PprofAddr
	if pprofAddr == "" {
		pprofAddr = os.Getenv("FLEETPKG_MCP_PPROF_ADDR")
	}
	if pprofAddr != "" {
		pprofMux := http.NewServeMux()
		pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
		pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		pprofListener, err := net.Listen("tcp", pprofAddr)
		if err != nil {
			return fmt.Errorf("failed to listen for pprof on %q: %w", pprofAddr, err)
		}
		go func() {
			<-ctx.Done()
			pprofListener.Close()
		}()
		go http.Serve(pprofListener, pprofMux)
		log.Info("pprof debug server listening", slog.String("addr", "http://"+pprofListener.Addr().String()+"/debug/pprof/"))
	}

	// Create atomic DB pointer for lazy initialization
	dbPtr := &atomic.Pointer[sql.DB]{}

	// Create MCP server immediately
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "fleetpkg",
		Title:   "Elastic Fleet Integration Package metadata MCP server",
		Version: modVer + " (" + vcsRef + ")",
	}, nil)
	fleetmcp.AddTools(s, append(pkgsql.TableSchemas(), ECSTableSchemas()...), dbPtr, log, metrics)
	fleetmcp.AddPrompts(s)

	// Track database file path for cleanup.
	var dbPath string
	var dbPathMu sync.Mutex

	// Parse refresh interval from flag or environment variable.
	var refreshInterval time.Duration
	refreshStr := cfg.Refresh
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
		if cfg.Refresh == "" {
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
		if cfg.GitPull {
			if err := ensureGitRepo(ctx, log, cfg.IntegrationsDir); err != nil {
				log.Error("Failed to ensure git repository", slog.Any("error", err))
				initErrCh <- fmt.Errorf("failed to ensure git repository: %w", err)
				return
			}
		}
		log.Info("Starting database initialization...")
		db, path, err := InitializeDatabase(ctx, log, cfg.IntegrationsDir)
		duration := time.Since(start)
		if err != nil {
			log.Error("Database initialization failed", slog.Any("error", err))
			metrics.RecordError(ctx, "db_init")
			initErrCh <- err
			return
		}
		dbPtr.Store(db)
		dbPathMu.Lock()
		dbPath = path
		dbPathMu.Unlock()
		metrics.RecordDBInit(ctx, duration)
		log.Info("Database initialization completed", slog.Duration("duration", duration), slog.String("path", path))
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
					if cfg.GitPull {
						if err := ensureGitRepo(ctx, log, cfg.IntegrationsDir); err != nil {
							log.Error("Failed to update git repository before refresh", slog.Any("error", err))
						}
					}
					log.Info("Starting periodic database refresh...")
					start := time.Now()
					newDB, err := RefreshDatabase(ctx, log, path, cfg.IntegrationsDir)
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
	if cfg.HTTPAddr != "" {
		mcpHandler := mcp.NewStreamableHTTPHandler(
			func(r *http.Request) *mcp.Server { return s },
			&mcp.StreamableHTTPOptions{
				Stateless: true,
			},
		)

		// Register health check and MCP endpoints.
		mux := http.NewServeMux()
		mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok\n"))
		})
		mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
			if dbPtr.Load() == nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("database not ready\n"))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok\n"))
		})
		mux.Handle("/", mcpHandler)

		var handler http.Handler = mux

		listener, err := net.Listen("tcp", cfg.HTTPAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on %q: %w", cfg.HTTPAddr, err)
		}
		go func() {
			<-ctx.Done()
			listener.Close()
		}()

		log.Info("fleetpkg-mcp handler listening",
			slog.String("addr", "http://"+listener.Addr().String()))

		// Add middleware chain (innermost to outermost):
		// 1. Mux with health checks + MCP handler (innermost)
		// 2. User context middleware (extracts headers)
		// 3. Metrics middleware (records request metrics)
		// 4. Logging handler (outermost, optional)
		handler = userContextMiddleware(handler)
		handler = metricsMiddleware(metrics, handler)

		if !cfg.NoLog {
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

func newLogger(logLevel string, sink io.Writer) (*slog.Logger, error) {
	level := new(slog.LevelVar)
	if err := level.UnmarshalText([]byte(logLevel)); err != nil {
		return nil, err
	}

	textHandler := slog.NewTextHandler(sink, &slog.HandlerOptions{
		Level: level,
	})

	// Wrap with UserContextHandler to automatically add user info from context.
	userContextHandler := slogutil.NewUserContextHandler(textHandler)

	return slog.New(userContextHandler), nil
}
