package atlas

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParityForcedColorCommandSweep(t *testing.T) {
	t.Setenv("FORCE_COLOR", "3")
	t.Setenv("NO_COLOR", "")
	reference := parityReference(t)
	left, right := parityProjects(t)
	for _, dir := range []string{left, right} {
		_, _, err := InitStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, domain := range []string{"code", "empty"} {
			if err := AddDomain(dir, domain); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "src", "a.go"), []byte("package a\n"), 0644); err != nil {
			t.Fatal(err)
		}
		path, _ := ExpertisePath(dir, "code")
		records := []Record{
			{"id": "mx-abc001", "type": "pattern", "name": "Locking", "description": "Lock every record before changing shared state", "classification": "foundational", "recorded_at": "2026-01-01T00:00:00.000Z", "files": []string{"src/a.go"}},
			{"id": "mx-abc002", "type": "failure", "description": "Shared temp file race", "resolution": "Use isolated temporary folders", "classification": "foundational", "recorded_at": "2026-01-02T00:00:00.000Z"},
		}
		if err := WriteRecords(path, records); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Atlas compatibility test"}, {"config", "user.email", "atlas-test@example.invalid"}, {"add", "."}, {"commit", "-qm", "baseline"}} {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %s %v", args, output, err)
			}
		}
		records = append(records, Record{"id": "mx-abc003", "type": "convention", "content": "Never change shared records without a lock", "classification": "foundational", "recorded_at": "2026-01-03T00:00:00.000Z"})
		if err := WriteRecords(path, records); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "src", "a.go"), []byte("package a\n// changed\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	steps := []parityStep{{Args: []string{"--help"}}}
	for _, spec := range contract.CLI.Commands {
		steps = append(steps, parityStep{Args: []string{spec.Name, "--help"}})
	}
	for _, args := range [][]string{{"query", "code"}, {"search", "lock"}, {"show", "mx-abc001"}, {"rank"}, {"ready"}, {"status"}, {"stale"}, {"validate"}, {"doctor"}, {"audit"}, {"audit", "--suggest"}, {"learn"}, {"diff", "--since", "HEAD"}, {"sync"}, {"sync"}, {"add", "newdomain"}} {
		steps = append(steps, parityStep{Args: args})
	}
	for _, step := range steps {
		t.Run(strings.Join(step.Args, " "), func(t *testing.T) {
			cmd := exec.Command("bun", append([]string{reference}, step.Args...)...)
			cmd.Dir = left
			cmd.Env = append(os.Environ(), "FORCE_COLOR=3", "LUUNA_ATLAS_LOG_JSON=1")
			cmd.Stdin = strings.NewReader(step.Input)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			code := 0
			if exit, ok := err.(*exec.ExitError); ok {
				code = exit.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			ref := parityResult{Code: code, Out: stdout.String(), Err: stderr.String()}
			got := parityRun(t, reference, right, step, true)
			parityCompare(t, strings.Join(step.Args, " "), ref, got, left, right, time.Now())
		})
	}
	t.Run("NO_COLOR_overrides_forced_help", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		cmd := exec.Command("bun", reference, "--help")
		cmd.Dir = left
		cmd.Env = os.Environ()
		stdout, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		got := parityRun(t, reference, right, parityStep{Args: []string{"--help"}}, true)
		if string(stdout) != got.Out || strings.Contains(got.Out, "\x1b[") {
			t.Fatalf("NO_COLOR help mismatch\nref=%q\ngo=%q", stdout, got.Out)
		}
	})
}
