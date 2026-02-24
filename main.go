// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/andrewkroh/fleetpkg-mcp/internal/app"
)

func main() {
	cfg := app.Config{LogLevel: "info"}

	flag.StringVar(&cfg.HTTPAddr, "http", "", "listen for HTTP at this address, instead of stdin/stdout")
	flag.StringVar(&cfg.PprofAddr, "pprof", "", "listen for pprof debug HTTP at this address (e.g., 127.0.0.1:6060)")
	flag.BoolVar(&cfg.NoLog, "no-log", false, "if set, disables logging")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "log level (debug, info, warn, error)")
	flag.StringVar(&cfg.IntegrationsDir, "dir", "", "path to elastic/integrations directory")
	flag.StringVar(&cfg.Refresh, "refresh", "", "periodically refresh database at this interval (e.g., 1h, 30m)")
	flag.BoolVar(&cfg.GitPull, "git-pull", false, "run 'git pull' on the integrations directory before each database load")
	version := flag.Bool("version", false, "print version and exit")

	flag.Parse()

	if *version {
		fmt.Println(app.BuildVersion())
		return
	}

	if cfg.IntegrationsDir == "" {
		fmt.Fprintln(os.Stderr, "ERROR: -dir flag is required")
		fmt.Fprintln(os.Stderr, "Example: -dir /data/integrations")
		os.Exit(2)
	}

	// Warn if -git-pull not set and dir doesn't exist
	if !cfg.GitPull {
		if _, err := os.Stat(cfg.IntegrationsDir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "ERROR: directory %q does not exist and -git-pull is not enabled\n", cfg.IntegrationsDir)
			fmt.Fprintln(os.Stderr, "Either create the directory with integrations data, or use -git-pull to clone automatically")
			os.Exit(2)
		}
	}

	if err := app.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
