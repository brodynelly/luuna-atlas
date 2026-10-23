package atlas

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFrozenReferenceIntegrity(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "..", "..", "compat", "reference")
	data, err := os.ReadFile(filepath.Join(root, "integrity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Baseline string
		Files    map[string]string
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Baseline != "27477c313a3049b1579dfdc32c39da600726721f" || len(manifest.Files) != 83 {
		t.Fatalf("unexpected frozen reference manifest: %s, %d files", manifest.Baseline, len(manifest.Files))
	}
	for name, expected := range manifest.Files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Error(err)
			continue
		}
		// Git checkouts may use CRLF on Windows and LF on Unix.
		digest := sha256.Sum256([]byte(strings.ReplaceAll(string(data), "\r\n", "\n")))
		if hex.EncodeToString(digest[:]) != expected {
			t.Errorf("frozen reference changed: %s", name)
		}
	}
}
