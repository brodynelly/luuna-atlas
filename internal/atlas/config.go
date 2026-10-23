package atlas

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const GitAttributesLine = ".luuna/atlas/expertise/*.jsonl merge=union"

var domainNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)
var apiDefaults struct {
	sync.RWMutex
	config Config
}

func DefaultConfig() Config {
	apiDefaults.RLock()
	override := apiDefaults.config
	apiDefaults.RUnlock()
	if override != nil {
		return Config(Obj(cloneTransportValue(override)))
	}
	cfg := Config{"version": "1", "domains": map[string]any{}, "governance": map[string]any{"max_entries": 100, "warn_entries": 150, "hard_limit": 200}, "classification_defaults": map[string]any{"shelf_life": map[string]any{"tactical": 14, "observational": 30}}}
	rememberOrder(map[string]any(cfg), []string{"version", "domains", "governance", "classification_defaults"})
	rememberOrder(Obj(cfg["governance"]), []string{"max_entries", "warn_entries", "hard_limit"})
	rememberOrder(Obj(Obj(cfg["classification_defaults"])["shelf_life"]), []string{"tactical", "observational"})
	return cfg
}
func ValidateDomainName(domain string) error {
	if !domainNameRE.MatchString(domain) {
		return fmt.Errorf("Invalid domain name: \"%s\". Only alphanumeric characters, hyphens, and underscores are allowed.", domain)
	}
	return nil
}
func exists(path string) bool  { _, e := os.Stat(path); return e == nil }
func lexists(path string) bool { _, e := os.Lstat(path); return e == nil }
func gitCommonDir(cwd string) string {
	raw, e := Git(cwd, "rev-parse", "--git-common-dir")
	if e != nil || strings.TrimSpace(raw) == "" {
		return ""
	}
	raw = strings.TrimSpace(raw)
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(cwd, raw)
	}
	abs, _ := filepath.Abs(raw)
	return abs
}
func IsInsideWorktree(cwd string) bool {
	common := gitCommonDir(cwd)
	if common == "" || !strings.HasSuffix(common, ".git") {
		return false
	}
	abs, _ := filepath.Abs(cwd)
	return filepath.Clean(filepath.Dir(common)) != filepath.Clean(abs)
}
func resolveStoreRoot(cwd string) string {
	common := gitCommonDir(cwd)
	if common == "" {
		return cwd
	}
	root := filepath.Dir(common)
	if !strings.HasSuffix(common, ".git") {
		root = filepath.Dir(root)
	}
	abs, _ := filepath.Abs(cwd)
	if filepath.Clean(root) == filepath.Clean(abs) {
		return cwd
	}
	if exists(filepath.Join(root, ".luuna", "atlas", "config.yaml")) || HasLegacyStore(root) {
		return root
	}
	return cwd
}
func AtlasDir(cwd string) (string, error) {
	root := resolveStoreRoot(cwd)
	if err := MigrateLegacyStore(root); err != nil {
		return "", err
	}
	return filepath.Join(root, ".luuna", "atlas"), nil
}
func ConfigPath(cwd string) (string, error) {
	dir, e := AtlasDir(cwd)
	return filepath.Join(dir, "config.yaml"), e
}
func ExpertisePath(cwd, domain string) (string, error) {
	if e := ValidateDomainName(domain); e != nil {
		return "", e
	}
	dir, e := AtlasDir(cwd)
	return filepath.Join(dir, "expertise", domain+".jsonl"), e
}
func ArchivePath(cwd, domain string) (string, error) {
	if e := ValidateDomainName(domain); e != nil {
		return "", e
	}
	dir, e := AtlasDir(cwd)
	return filepath.Join(dir, "archive", domain+".jsonl"), e
}
func ReadConfig(cwd string) (Config, error) {
	path, e := ConfigPath(cwd)
	if e != nil {
		return nil, e
	}
	data, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return nil, fmt.Errorf("No .luuna/atlas/ directory found. Run `luuna-atlas init` to set up this project.")
	}
	if e != nil {
		return nil, e
	}
	var node yaml.Node
	if e = yaml.Unmarshal(data, &node); e != nil {
		return nil, fmt.Errorf("Failed to parse config.yaml: %s. Check the YAML syntax.", e)
	}
	if e = validateYAMLKeys(&node, string(data)); e != nil {
		return nil, fmt.Errorf("Failed to parse config.yaml: %s. Check the YAML syntax.", e)
	}
	raw := map[string]any{}
	if len(node.Content) > 0 {
		raw = Obj(yamlValue(node.Content[0]))
	}
	cfg := Config(raw)
	cfgOrder := ObjectKeys(cfg)
	domains := map[string]any{}
	switch v := cfg["domains"].(type) {
	case []any:
		keys := []string{}
		for _, x := range v {
			if s, ok := x.(string); ok {
				domains[s] = map[string]any{}
				keys = append(keys, s)
			}
		}
		rememberOrder(domains, keys)
	case map[string]any:
		domains = v
	}
	cfg["domains"] = domains
	cfgOrder = Unique(append(cfgOrder, "domains"))
	defaults := DefaultConfig()
	gov := Obj(defaults["governance"])
	userGov := Obj(cfg["governance"])
	govOrder := ObjectKeys(gov)
	for _, k := range ObjectKeys(userGov) {
		gov[k] = userGov[k]
	}
	cfg["governance"] = gov
	rememberOrder(gov, Unique(append(govOrder, ObjectKeys(userGov)...)))
	cd := Obj(defaults["classification_defaults"])
	cdOrder := ObjectKeys(cd)
	userCD := Obj(cfg["classification_defaults"])
	shelf := Obj(cd["shelf_life"])
	shelfOrder := ObjectKeys(shelf)
	userShelf := Obj(userCD["shelf_life"])
	for _, k := range ObjectKeys(userCD) {
		cd[k] = userCD[k]
	}
	for _, k := range ObjectKeys(userShelf) {
		shelf[k] = userShelf[k]
	}
	cd["shelf_life"] = shelf
	rememberOrder(cd, Unique(append(cdOrder, ObjectKeys(userCD)...)))
	cfg["classification_defaults"] = cd
	rememberOrder(shelf, Unique(append(shelfOrder, ObjectKeys(userShelf)...)))
	rememberOrder(raw, Unique(append(cfgOrder, "governance", "classification_defaults")))
	return cfg, nil
}
func yamlValue(n *yaml.Node) any {
	switch n.Kind {
	case yaml.MappingNode:
		m := map[string]any{}
		keys := []string{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Tag == "!!merge" {
				merge := yamlValue(n.Content[i+1])
				sources := []any{merge}
				if sequence, ok := merge.([]any); ok {
					sources = sequence
				}
				for _, source := range sources {
					object := Obj(source)
					for _, k := range ObjectKeys(object) {
						if _, exists := m[k]; !exists {
							m[k] = object[k]
							keys = append(keys, k)
						}
					}
				}
				continue
			}
			k := yamlMappingKey(n.Content[i])
			m[k] = yamlValue(n.Content[i+1])
			keys = append(keys, k)
		}
		rememberOrder(m, keys)
		return m
	case yaml.SequenceNode:
		a := []any{}
		for _, v := range n.Content {
			a = append(a, yamlValue(v))
		}
		return a
	case yaml.AliasNode:
		return yamlValue(n.Alias)
	}
	var v any
	if e := n.Decode(&v); e != nil {
		return n.Value
	}
	if t, ok := v.(interface{ Format(string) string }); ok {
		return t.Format("2006-01-02T15:04:05.000Z")
	}
	return v
}
func YAMLValue(data string) (any, error) {
	var n yaml.Node
	if e := yaml.Unmarshal([]byte(data), &n); e != nil {
		return nil, e
	}
	if e := validateYAMLKeys(&n, data); e != nil {
		return nil, e
	}
	if len(n.Content) == 0 {
		return nil, nil
	}
	return yamlValue(n.Content[0]), nil
}
func yamlNode(v any) *yaml.Node {
	switch x := v.(type) {
	case Config:
		return yamlNode(map[string]any(x))
	case Record:
		return yamlNode(map[string]any(x))
	case map[string]any:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, k := range ObjectKeys(x) {
			n.Content = append(n.Content, yamlNode(k), yamlNode(x[k]))
		}
		return n
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range x {
			n.Content = append(n.Content, yamlNode(item))
		}
		return n
	case []string:
		a := make([]any, len(x))
		for i, s := range x {
			a[i] = s
		}
		return yamlNode(a)
	}
	n := &yaml.Node{}
	_ = n.Encode(v)
	if text, ok := v.(string); ok && !strings.ContainsAny(text, "\x00\t\r\n") {
		if n.Style == yaml.DoubleQuotedStyle || regexp.MustCompile(`(?i)^(y|yes|n|no|true|false|on|off)$`).MatchString(text) {
			n.Style = yaml.SingleQuotedStyle
		}
	}
	return n
}
func MarshalYAML(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	e := enc.Encode(yamlNode(v))
	if e != nil {
		return nil, e
	}
	_ = enc.Close()
	return b.Bytes(), nil
}
func WriteConfig(cwd string, cfg Config) error {
	path, e := ConfigPath(cwd)
	if e != nil {
		return e
	}
	b, e := MarshalYAML(cfg)
	if e != nil {
		return e
	}
	return os.WriteFile(path, b, 0666)
}
func AddDomain(cwd, domain string) error {
	if e := ValidateDomainName(domain); e != nil {
		return e
	}
	cfg, e := ReadConfig(cwd)
	if e != nil {
		return e
	}
	domains := Obj(cfg["domains"])
	if _, ok := domains[domain]; !ok {
		keys := ObjectKeys(domains)
		domains[domain] = map[string]any{}
		rememberOrder(domains, append(keys, domain))
		if e = WriteConfig(cwd, cfg); e != nil {
			return e
		}
	}
	path, e := ExpertisePath(cwd, domain)
	if e != nil {
		return e
	}
	if !exists(path) {
		return os.WriteFile(path, []byte{}, 0666)
	}
	return nil
}
func RemoveDomain(cwd, domain string) error {
	if e := ValidateDomainName(domain); e != nil {
		return e
	}
	cfg, e := ReadConfig(cwd)
	if e != nil {
		return e
	}
	domains := Obj(cfg["domains"])
	if _, ok := domains[domain]; !ok {
		return fmt.Errorf("Domain \"%s\" not found in config.", domain)
	}
	delete(domains, domain)
	if e = WriteConfig(cwd, cfg); e != nil {
		return e
	}
	path, e := ExpertisePath(cwd, domain)
	if e != nil {
		return e
	}
	if exists(path) {
		return os.Remove(path)
	}
	return nil
}
func InitStore(cwd string) (bool, string, error) {
	dir, e := AtlasDir(cwd)
	if e != nil {
		return false, "", e
	}
	created := !exists(dir)
	if e = os.MkdirAll(filepath.Join(dir, "expertise"), 0777); e != nil {
		return false, dir, e
	}
	path := filepath.Join(dir, "config.yaml")
	if !exists(path) {
		if e = os.WriteFile(path, []byte(InitialConfigYAML), 0666); e != nil {
			return false, dir, e
		}
	}
	attrs := filepath.Join(cwd, ".gitattributes")
	old, _ := os.ReadFile(attrs)
	if !strings.Contains(string(old), GitAttributesLine) {
		sep := ""
		if len(old) > 0 && !bytes.HasSuffix(old, []byte("\n")) {
			sep = "\n"
		}
		if e = os.WriteFile(attrs, []byte(string(old)+sep+GitAttributesLine+"\n"), 0666); e != nil {
			return false, dir, e
		}
	}
	readme := filepath.Join(dir, "README.md")
	if !exists(readme) {
		if e = os.WriteFile(readme, []byte(AtlasReadme), 0666); e != nil {
			return false, dir, e
		}
	}
	return created, dir, nil
}

// Initial strings are captured from the current reference source, preserving user-facing scaffolding.
const InitialConfigYAML = "# Atlas configuration. See https://github.com/brodynelly/luuna-atlas for docs.\n#\n# Required fields below. Optional knobs are commented out at the bottom — uncomment\n# to enable. Note: commands that mutate this file (e.g. `luuna-atlas add <domain>`,\n# `luuna-atlas record` auto-creating a domain) rewrite it via the YAML serializer and\n# strip these comments.\n\nversion: '1'\ndomains: {}\ngovernance:\n  max_entries: 100\n  warn_entries: 150\n  hard_limit: 200\nclassification_defaults:\n  shelf_life:\n    tactical: 14\n    observational: 30\n\n# ─── Optional knobs (uncomment to enable) ────────────────────────────────────\n#\n# prime:\n#   # Pin `luuna-atlas prime`'s unscoped output shape. When unset (recommended), prime\n#   # emits a budget-capped record index (one line per record, failures first).\n#   #   - full     → the record index (same as unset)\n#   #   - manifest → a per-domain count table instead of records\n#   # --full / --manifest / scoping flags always override this on a per-call basis.\n#   default_mode: full        # one of: full, manifest\n#   # Trust-tier ranking weights for full-mode output. Sort score per record =\n#   # (★ count * star) + classification weight. Override only the knobs you\n#   # want to retune; unset fields keep their default. Higher scores surface\n#   # first; within-tier ties preserve insertion order.\n#   tier_weights:\n#     star: 100               # multiplier on the ★ confirmation count\n#     foundational: 50        # base score for foundational records\n#     tactical: 20             # base score for tactical records\n#     observational: 10        # base score for observational records\n#\n# search:\n#   # Multiplier applied to BM25 scores so records with more confirmed outcomes\n#   # rank higher. 0 disables (pure BM25). Default 0.1: a record with N successes\n#   # gets a (1 + 0.1*N) boost. Override per-call with `luuna-atlas search --no-boost`.\n#   boost_factor: 0.1\n#\n# disabled_types:\n#   # Names of registered types (built-in or custom) that emit a deprecation\n#   # warning on write. Reads still succeed; the type stays in CLI choices so\n#   # peers in shared domains don't hard-fail. Use to retire a type gracefully.\n#   - failure\n#\n# custom_types:\n#   # Project-specific record types. Required + optional fields, dedup_key,\n#   # and a summary template. See README for the full schema.\n#   hypothesis:\n#     required: [statement, prediction]\n#     optional: [evidence_files]\n#     dedup_key: statement\n#     summary: \"{statement}\"   # tokens use single braces; {{statement}} also accepted\n#     # aliases: rename a field while still reading legacy on-disk records.\n#     # Map canonical (current) name → list of legacy names. At read time,\n#     # legacy fields are rewritten to canonical and dropped from the record.\n#     aliases:\n#       statement: [claim]\n#   # Inherit from a built-in type with extends:. Required/optional arrays\n#   # merge with the parent's (union); dedup_key, summary, compact, section_title,\n#   # extracts_files, and files_field override only when set, otherwise inherit.\n#   # Custom-from-custom is not supported in v1.\n#   adr:\n#     extends: decision\n#     required: [decision_status, deciders]   # added on top of decision's [title, rationale]\n#     summary: \"{decision_status}: {title}\"   # tokens must be declared fields (parent's included)\n#\n# quality:\n#   # Write-time gate on luuna-atlas record: near-duplicates, conventions/patterns with\n#   # no file/dir anchor or evidence, conventions that restate code.\n#   # warn (default) prints and writes; error blocks unless --force; off skips.\n#   # Exact duplicates are always blocked.\n#   level: warn\n#\n# hooks:\n#   # Lifecycle hook scripts. Each event maps to an ordered list of shell\n#   # commands. Atlas invokes each with the relevant payload as JSON on stdin.\n#   # Exit 0 = continue. Non-zero from a `pre-*` hook blocks the action; from\n#   # a `post-*` hook emits a warning. `pre-record`, `pre-prime`, and\n#   # `pre-compact` may mutate the payload by printing modified JSON on\n#   # stdout; `pre-prune` is block-or-allow only — its stdout is ignored.\n#   #\n#   # pre-record:    [./.luuna/atlas/hooks/scan-secrets.sh, ./.luuna/atlas/hooks/require-owner.sh]\n#   # post-record:   [./.luuna/atlas/hooks/post-to-slack.sh]\n#   # pre-prime:     [./.luuna/atlas/hooks/filter-by-team.sh]\n#   # pre-prune:     [./.luuna/atlas/hooks/digest-then-confirm.sh]\n#   # pre-compact:   [./.luuna/atlas/hooks/llm-summarize.sh]\n#\n# hook_settings:\n#   # Per-hook execution timeout in milliseconds. Default 5000.\n#   timeout_ms: 5000\n"
const AtlasReadme = "# .luuna/atlas/\n\nThis directory is managed by [luuna-atlas](https://github.com/brodynelly/luuna-atlas) — a structured expertise layer for coding agents.\n\n## Key Commands\n\n- `luuna-atlas init`      — Initialize a .luuna/atlas directory\n- `luuna-atlas add`       — Add a new domain\n- `luuna-atlas record`    — Record an expertise record\n- `luuna-atlas edit`      — Edit an existing record\n- `luuna-atlas query`     — Query expertise records\n- `luuna-atlas prime [domain]` — Output a record index for priming (optionally scoped to one domain)\n- `luuna-atlas show <id>`  — Show a full record\n- `luuna-atlas search`   — Search records across domains\n- `luuna-atlas status`    — Show domain statistics\n- `luuna-atlas validate`  — Validate all records against the schema\n- `luuna-atlas prune`     — Remove expired records\n\n## Structure\n\n- `config.yaml` — Configuration file\n- `expertise/`        — JSONL files, one per domain\n\n## Configuration\n\nOptional knobs in `config.yaml`:\n\n```yaml\nprime:\n  default_mode: manifest   # or \"full\" (the default): a budget-capped record\n                           # index, failures first. \"manifest\" lists per-domain\n                           # counts instead. Scoping flags (`--files`,\n                           # `<domain>`) always emit records.\n\nsearch:\n  boost_factor: 0.1        # multiplier on BM25 scores for confirmed records.\n                           # 0 disables (pure BM25). Override with\n                           # `luuna-atlas search --no-boost`.\n```\n"
