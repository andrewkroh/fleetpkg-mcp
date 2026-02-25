// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// gitEnv returns environment variables for running git commands in
// automated/containerized environments. It disables interactive prompts
// and automatic garbage collection (which can fork background processes
// that become zombies when the Go process runs as PID 1).
func gitEnv() []string {
	return append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",                // Disable HTTPS credential prompts.
		"GIT_SSH_COMMAND=ssh -o BatchMode=yes", // Disable SSH passphrase/password prompts.
		"GIT_CONFIG_COUNT=1",                   // Disable gc to prevent zombie child processes.
		"GIT_CONFIG_KEY_0=gc.auto",
		"GIT_CONFIG_VALUE_0=0",
	)
}

// gitPullRepo runs 'git pull --ff-only' in the given directory.
func gitPullRepo(ctx context.Context, log *slog.Logger, dir string) error {
	log.Info("Running git pull...", slog.String("dir", dir))
	start := time.Now()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "pull", "--ff-only")
	cmd.Env = gitEnv()
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
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "clone",
		"--depth=1",
		"--single-branch",
		"--no-tags",
		"https://github.com/elastic/integrations.git",
		dir)
	cmd.Env = gitEnv()

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
