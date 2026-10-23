package atlas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureGit(t *testing.T) string {
	t.Helper()
	cwd := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.email", "atlas-test@example.invalid"}, {"config", "user.name", "Atlas Test"}} {
		if _, e := Git(cwd, args...); e != nil {
			t.Fatal(e)
		}
	}
	return cwd
}
func commitFixture(t *testing.T, cwd, message string) {
	t.Helper()
	if _, e := Git(cwd, "add", "."); e != nil {
		t.Fatal(e)
	}
	if _, e := Git(cwd, "commit", "-m", message); e != nil {
		t.Fatal(e)
	}
}
func TestNativeWorktreeStoreIdentity(t *testing.T) {
	cwd := fixtureGit(t)
	_ = os.WriteFile(filepath.Join(cwd, "README.md"), []byte("fixture\n"), 0666)
	commitFixture(t, cwd, "initial")
	if _, _, e := InitStore(cwd); e != nil {
		t.Fatal(e)
	}
	worktree := filepath.Join(t.TempDir(), "linked")
	if _, e := Git(cwd, "worktree", "add", "-b", "worker", worktree); e != nil {
		t.Fatal(e)
	}
	defer Git(cwd, "worktree", "remove", "--force", worktree)
	root, e := AtlasDir(worktree)
	if e != nil {
		t.Fatal(e)
	}
	actualInfo, actualErr := os.Stat(root)
	wantInfo, wantErr := os.Stat(filepath.Join(cwd, ".luuna", "atlas"))
	if actualErr != nil || wantErr != nil || !os.SameFile(actualInfo, wantInfo) {
		t.Fatalf("worktree pointed at %s; wanted %s", root, filepath.Join(cwd, ".luuna", "atlas"))
	}
	if !IsInsideWorktree(worktree) || IsInsideWorktree(cwd) {
		t.Fatal("worktree identity incorrect")
	}
	if e = AddDomain(worktree, "shared"); e != nil {
		t.Fatal(e)
	}
	if !exists(filepath.Join(cwd, ".luuna", "atlas", "expertise", "shared.jsonl")) {
		t.Fatal("write did not use main store")
	}
}
func TestNativeGitAnchorChurn(t *testing.T) {
	cwd := fixtureGit(t)
	file := filepath.Join(cwd, "anchor.txt")
	_ = os.WriteFile(file, []byte("initial\n"), 0666)
	commitFixture(t, cwd, "initial")
	sha := GetCurrentCommit(cwd)
	for n := 1; n <= 6; n++ {
		_ = os.WriteFile(file, []byte(fmt.Sprintf("change %d\n", n)), 0666)
		commitFixture(t, cwd, fmt.Sprintf("change %d", n))
	}
	r := Record{"id": "mx-abcdef", "type": "pattern", "name": "anchor", "description": "known pattern", "classification": "foundational", "recorded_at": ISO(time.Now().Add(-time.Minute)), "files": []string{"anchor.txt"}, "evidence": map[string]any{"commit": sha}}
	entries := FindStaleRecords([]Record{r}, cwd, time.Now(), map[string]any{"tactical": 14, "observational": 30})
	if len(entries) != 1 || len(entries[0].Reasons) != 1 || entries[0].Reasons[0].Kind != "anchors_changed" || !strings.Contains(entries[0].Reasons[0].Detail, "6 commits") {
		t.Fatalf("churn evidence mismatch: %v", entries)
	}
	if !PrimeStaleIDs(entries)["mx-abcdef"] {
		t.Fatal("prime stale marker missing")
	}
}
func TestNativeAnchorMatchingBoundary(t *testing.T) {
	if FileMatchesAny("cli.ts", []string{"other-cli.ts"}) {
		t.Fatal("suffix crossed path segment")
	}
	if !FileMatchesAny("cli.ts", []string{"src/cli.ts"}) {
		t.Fatal("segment suffix not matched")
	}
	if _, e := NormalizeDirAnchor("../outside"); e == nil {
		t.Fatal("parent anchor accepted")
	}
	if _, e := NormalizeDirAnchor(`C:\outside`); e == nil {
		t.Fatal("absolute anchor accepted")
	}
	if got := InferDirAnchors([]string{"src/a.ts", "src/b.ts", "src/c.ts"}); len(got) != 1 || got[0] != "src" {
		t.Fatalf("inferred %v", got)
	}
}
