package atlas

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"
)

func TestNativeUnicodeNormalizationAgainstJavaScript(t *testing.T) {
	reference := filepath.Join(filepath.Dir(parityReference(t)), "utils", "expertise.ts")
	texts := []string{"ΟΣ", "ΟΣΑ", "ΌΣ", "AΣ́", "AΣB", "İstanbul", "AİB", "İ", "Σ", "𐐀Σ"}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if unicode.ToLower(r) != r {
			texts = append(texts, string(r))
		}
	}
	input, _ := json.Marshal(texts)
	script := `const { normalizeText } = await import(process.env.ATLAS_NORMALIZE_REFERENCE); const input = await Bun.stdin.json(); console.log(JSON.stringify(input.map(normalizeText)));`
	command := exec.Command("bun", "--eval", script)
	command.Env = append(os.Environ(), "ATLAS_NORMALIZE_REFERENCE="+reference)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Unicode oracle: %v %s", err, output)
	}
	expected := []string{}
	if err = json.Unmarshal(output, &expected); err != nil {
		t.Fatal(err)
	}
	for i, input := range texts {
		if got := NormalizeText(input); got != expected[i] {
			t.Errorf("JS lowercase/normalization mismatch input %q: want %q, got %q", input, expected[i], got)
		}
	}
	t.Logf("Compared %d uppercase/full/contextual Unicode cases against the original Bun implementation", len(texts))
}

func TestNativeYAMLDuplicateDiagnosticAgainstJavaScript(t *testing.T) {
	reference := parityReference(t)
	refDir, goDir := parityProjects(t)
	body := "version: '1'\ndomains: {}\ngovernance:\n  max_entries: 42\n  max_entries: 55\n"
	for _, dir := range []string{refDir, goDir} {
		parityPrimeFixture(t, dir)
		path, _ := ConfigPath(dir)
		if err := os.WriteFile(path, []byte(body), 0666); err != nil {
			t.Fatal(err)
		}
	}
	step := parityStep{Args: []string{"config", "show"}}
	want := parityRun(t, reference, refDir, step, false)
	got := parityRun(t, reference, goDir, step, true)
	parityCompare(t, "duplicate YAML caught diagnostic", want, got, refDir, goDir, time.Now())
	uncaught := parityRun(t, reference, refDir, parityStep{Args: []string{"status", "--json"}}, false)
	native := parityRun(t, reference, goDir, parityStep{Args: []string{"status", "--json"}}, true)
	report := "BUN FULL STDERR:\n" + uncaught.Err + "\nNATIVE STDERR:\n" + native.Err
	if err := os.WriteFile(filepath.Join(os.TempDir(), "atlas-yaml-bun-full.log"), []byte(report), 0666); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(uncaught.Err, "duplicated mapping key") {
		t.Fatal("missing duplicate mapping diagnostic")
	}
}
