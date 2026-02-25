// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package app

import (
	"os/exec"
	"strings"
	"testing"
)

// TestGitEnvDisablesGC verifies that gitEnv configures git to skip
// automatic garbage collection.
func TestGitEnvDisablesGC(t *testing.T) {
	env := gitEnv()

	want := map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "gc.auto",
		"GIT_CONFIG_VALUE_0": "0",
	}

	envMap := make(map[string]string)
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok {
			envMap[k] = v
		}
	}

	for k, v := range want {
		if got := envMap[k]; got != v {
			t.Errorf("gitEnv() %s = %q, want %q", k, got, v)
		}
	}
}

func gitRun(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// TestGitEnvGCAutoEffective verifies that git respects the gc.auto=0
// setting from gitEnv by checking the effective config value.
func TestGitEnvGCAutoEffective(t *testing.T) {
	cmd := exec.Command("git", "config", "--get", "gc.auto")
	cmd.Env = gitEnv()
	// git needs a repo context for config; use a temp dir.
	dir := t.TempDir()
	gitRun(t, "init", dir)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git config --get gc.auto: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != "0" {
		t.Errorf("git gc.auto = %q, want %q", got, "0")
	}
}

// TestGitEnvGCAutoOverrides verifies that gitEnv's gc.auto=0 takes
// precedence even if a repo-level gc.auto is configured.
func TestGitEnvGCAutoOverrides(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, "init", dir)
	gitRun(t, "-C", dir, "config", "gc.auto", "6700")

	cmd := exec.Command("git", "-C", dir, "config", "--get", "gc.auto")
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git config --get gc.auto: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != "0" {
		t.Errorf("gc.auto = %q even with gitEnv(); want %q", got, "0")
	}
}
