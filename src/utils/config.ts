import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import yaml from "js-yaml";
import type { AtlasConfig, DomainConfig } from "../schemas/config.ts";
import { DEFAULT_CONFIG } from "../schemas/config.ts";
import { createExpertiseFile } from "./expertise.ts";
import { hasLegacyStore, migrateLegacyStore } from "./legacy-layout.ts";

const LUUNA_DIR = ".luuna";
const ATLAS_DIR = ".luuna/atlas";
const CONFIG_FILE = "config.yaml";
const EXPERTISE_DIR = "expertise";

export const GITATTRIBUTES_LINE = ".luuna/atlas/expertise/*.jsonl merge=union";

const INIT_CONFIG_HEADER = `# Atlas configuration. See https://github.com/brodynelly/luuna-atlas for docs.
#
# Required fields below. Optional knobs are commented out at the bottom — uncomment
# to enable. Note: commands that mutate this file (e.g. \`luuna-atlas add <domain>\`,
# \`luuna-atlas record\` auto-creating a domain) rewrite it via the YAML serializer and
# strip these comments.

`;

const INIT_CONFIG_OPTIONAL_KNOBS = `
# ─── Optional knobs (uncomment to enable) ────────────────────────────────────
#
# prime:
#   # Pin \`luuna-atlas prime\`'s unscoped output shape. When unset (recommended), prime
#   # emits a budget-capped record index (one line per record, failures first).
#   #   - full     → the record index (same as unset)
#   #   - manifest → a per-domain count table instead of records
#   # --full / --manifest / scoping flags always override this on a per-call basis.
#   default_mode: full        # one of: full, manifest
#   # Trust-tier ranking weights for full-mode output. Sort score per record =
#   # (★ count * star) + classification weight. Override only the knobs you
#   # want to retune; unset fields keep their default. Higher scores surface
#   # first; within-tier ties preserve insertion order.
#   tier_weights:
#     star: 100               # multiplier on the ★ confirmation count
#     foundational: 50        # base score for foundational records
#     tactical: 20             # base score for tactical records
#     observational: 10        # base score for observational records
#
# search:
#   # Multiplier applied to BM25 scores so records with more confirmed outcomes
#   # rank higher. 0 disables (pure BM25). Default 0.1: a record with N successes
#   # gets a (1 + 0.1*N) boost. Override per-call with \`luuna-atlas search --no-boost\`.
#   boost_factor: 0.1
#
# disabled_types:
#   # Names of registered types (built-in or custom) that emit a deprecation
#   # warning on write. Reads still succeed; the type stays in CLI choices so
#   # peers in shared domains don't hard-fail. Use to retire a type gracefully.
#   - failure
#
# custom_types:
#   # Project-specific record types. Required + optional fields, dedup_key,
#   # and a summary template. See README for the full schema.
#   hypothesis:
#     required: [statement, prediction]
#     optional: [evidence_files]
#     dedup_key: statement
#     summary: "{statement}"   # tokens use single braces; {{statement}} also accepted
#     # aliases: rename a field while still reading legacy on-disk records.
#     # Map canonical (current) name → list of legacy names. At read time,
#     # legacy fields are rewritten to canonical and dropped from the record.
#     aliases:
#       statement: [claim]
#   # Inherit from a built-in type with extends:. Required/optional arrays
#   # merge with the parent's (union); dedup_key, summary, compact, section_title,
#   # extracts_files, and files_field override only when set, otherwise inherit.
#   # Custom-from-custom is not supported in v1.
#   adr:
#     extends: decision
#     required: [decision_status, deciders]   # added on top of decision's [title, rationale]
#     summary: "{decision_status}: {title}"   # tokens must be declared fields (parent's included)
#
# quality:
#   # Write-time gate on luuna-atlas record: near-duplicates, conventions/patterns with
#   # no file/dir anchor or evidence, conventions that restate code.
#   # warn (default) prints and writes; error blocks unless --force; off skips.
#   # Exact duplicates are always blocked.
#   level: warn
#
# hooks:
#   # Lifecycle hook scripts. Each event maps to an ordered list of shell
#   # commands. Atlas invokes each with the relevant payload as JSON on stdin.
#   # Exit 0 = continue. Non-zero from a \`pre-*\` hook blocks the action; from
#   # a \`post-*\` hook emits a warning. \`pre-record\`, \`pre-prime\`, and
#   # \`pre-compact\` may mutate the payload by printing modified JSON on
#   # stdout; \`pre-prune\` is block-or-allow only — its stdout is ignored.
#   #
#   # pre-record:    [./.luuna/atlas/hooks/scan-secrets.sh, ./.luuna/atlas/hooks/require-owner.sh]
#   # post-record:   [./.luuna/atlas/hooks/post-to-slack.sh]
#   # pre-prime:     [./.luuna/atlas/hooks/filter-by-team.sh]
#   # pre-prune:     [./.luuna/atlas/hooks/digest-then-confirm.sh]
#   # pre-compact:   [./.luuna/atlas/hooks/llm-summarize.sh]
#
# hook_settings:
#   # Per-hook execution timeout in milliseconds. Default 5000.
#   timeout_ms: 5000
`;

export function buildInitialConfigYaml(): string {
	const body = yaml.dump(DEFAULT_CONFIG, { lineWidth: -1 });
	return INIT_CONFIG_HEADER + body + INIT_CONFIG_OPTIONAL_KNOBS;
}

export const ATLAS_README = `# .luuna/atlas/

This directory is managed by [luuna-atlas](https://github.com/brodynelly/luuna-atlas) — a structured expertise layer for coding agents.

## Key Commands

- \`luuna-atlas init\`      — Initialize a .luuna/atlas directory
- \`luuna-atlas add\`       — Add a new domain
- \`luuna-atlas record\`    — Record an expertise record
- \`luuna-atlas edit\`      — Edit an existing record
- \`luuna-atlas query\`     — Query expertise records
- \`luuna-atlas prime [domain]\` — Output a record index for priming (optionally scoped to one domain)
- \`luuna-atlas show <id>\`  — Show a full record
- \`luuna-atlas search\`   — Search records across domains
- \`luuna-atlas status\`    — Show domain statistics
- \`luuna-atlas validate\`  — Validate all records against the schema
- \`luuna-atlas prune\`     — Remove expired records

## Structure

- \`config.yaml\` — Configuration file
- \`expertise/\`        — JSONL files, one per domain

## Configuration

Optional knobs in \`config.yaml\`:

\`\`\`yaml
prime:
  default_mode: manifest   # or "full" (the default): a budget-capped record
                           # index, failures first. "manifest" lists per-domain
                           # counts instead. Scoping flags (\`--files\`,
                           # \`<domain>\`) always emit records.

search:
  boost_factor: 0.1        # multiplier on BM25 scores for confirmed records.
                           # 0 disables (pure BM25). Override with
                           # \`luuna-atlas search --no-boost\`.
\`\`\`
`;

function gitCommonDir(cwd: string): string | null {
	try {
		const raw = execFileSync("git", ["rev-parse", "--git-common-dir"], {
			cwd,
			encoding: "utf-8",
			stdio: ["pipe", "pipe", "pipe"],
		}).trim();
		if (!raw) return null;
		return resolve(cwd, raw);
	} catch {
		return null;
	}
}

function resolveWorktreeRoot(cwd: string): string {
	const common = gitCommonDir(cwd);
	if (!common) return cwd;

	// .git/worktrees/<name> → strip to repo root; .git → already main
	const mainRoot = common.endsWith(".git") ? dirname(common) : dirname(dirname(common));
	const mainResolved = resolve(mainRoot);

	if (mainResolved === resolve(cwd)) return cwd;

	// Only redirect if main repo has a .luuna/atlas/ with config
	const mainAtlasConfig = join(mainResolved, ATLAS_DIR, CONFIG_FILE);
	if (existsSync(mainAtlasConfig) || hasLegacyStore(mainResolved)) {
		return mainResolved;
	}

	return cwd;
}

export function isInsideWorktree(cwd: string = process.cwd()): boolean {
	const common = gitCommonDir(cwd);
	if (!common) return false;

	// For actual worktrees, --git-common-dir always returns the main .git dir
	// (ends with ".git"). Submodules return /parent/.git/modules/<name> which
	// does NOT end with ".git" — those are not worktrees, avoid false positive.
	if (!common.endsWith(".git")) return false;

	const mainRoot = dirname(common);
	return resolve(mainRoot) !== resolve(cwd);
}

export function getAtlasDir(cwd: string = process.cwd()): string {
	const root = resolveWorktreeRoot(cwd);
	migrateLegacyStore(root, { luunaDir: LUUNA_DIR, storeRel: ATLAS_DIR, configFile: CONFIG_FILE });
	return join(root, ATLAS_DIR);
}

export function getConfigPath(cwd: string = process.cwd()): string {
	return join(getAtlasDir(cwd), CONFIG_FILE);
}

export function getExpertiseDir(cwd: string = process.cwd()): string {
	return join(getAtlasDir(cwd), EXPERTISE_DIR);
}

export function validateDomainName(domain: string): void {
	if (!/^[a-zA-Z0-9][a-zA-Z0-9_-]*$/.test(domain)) {
		throw new Error(
			`Invalid domain name: "${domain}". Only alphanumeric characters, hyphens, and underscores are allowed.`,
		);
	}
}

export function getExpertisePath(domain: string, cwd: string = process.cwd()): string {
	validateDomainName(domain);
	return join(getExpertiseDir(cwd), `${domain}.jsonl`);
}

// Legacy on-disk shape was `domains: [a, b]`; current shape is
// `domains: { a: {}, b: {} }`. Normalize both to the object map at read time so
// older configs keep working without user migration.
function normalizeDomains(raw: unknown): Record<string, DomainConfig> {
	if (Array.isArray(raw)) {
		const map: Record<string, DomainConfig> = {};
		for (const name of raw) {
			if (typeof name === "string") map[name] = {};
		}
		return map;
	}
	if (raw && typeof raw === "object") return raw as Record<string, DomainConfig>;
	return {};
}

// Backfill required-by-type top-level sections that a hand-written minimal
// config may omit. Schema marks `governance` and `classification_defaults` as
// required, but consumers (doctor, prune, status, compact, prime) destructure
// them directly and would otherwise crash with a TypeError on a config that
// only declares `domains:`. Shallow-merge so partial user overrides
// (e.g. `governance: { max_entries: 50 }`) keep the other defaults.
function applyConfigDefaults(parsed: AtlasConfig): AtlasConfig {
	const userGov = (parsed.governance ?? {}) as Partial<AtlasConfig["governance"]>;
	const userCD = (parsed.classification_defaults ?? {}) as Partial<
		AtlasConfig["classification_defaults"]
	>;
	const userShelf = (userCD.shelf_life ?? {}) as Partial<
		AtlasConfig["classification_defaults"]["shelf_life"]
	>;
	parsed.governance = { ...DEFAULT_CONFIG.governance, ...userGov };
	parsed.classification_defaults = {
		shelf_life: { ...DEFAULT_CONFIG.classification_defaults.shelf_life, ...userShelf },
	};
	return parsed;
}

export async function readConfig(cwd: string = process.cwd()): Promise<AtlasConfig> {
	const configPath = getConfigPath(cwd);
	let content: string;
	try {
		content = await readFile(configPath, "utf-8");
	} catch (err) {
		if ((err as NodeJS.ErrnoException).code === "ENOENT") {
			throw new Error(
				"No .luuna/atlas/ directory found. Run `luuna-atlas init` to set up this project.",
			);
		}
		throw err;
	}
	let parsed: AtlasConfig;
	try {
		parsed = (yaml.load(content) ?? {}) as AtlasConfig;
	} catch (err) {
		throw new Error(
			`Failed to parse config.yaml: ${err instanceof Error ? err.message : String(err)}. Check the YAML syntax.`,
		);
	}
	if (!parsed || typeof parsed !== "object") {
		// Empty file or scalar at top level — treat as empty object so defaults apply.
		parsed = {} as AtlasConfig;
	}
	parsed.domains = normalizeDomains(parsed.domains);
	return applyConfigDefaults(parsed);
}

export async function addDomain(domain: string, cwd: string = process.cwd()): Promise<void> {
	validateDomainName(domain);
	const config = await readConfig(cwd);
	if (!(domain in config.domains)) {
		config.domains[domain] = {};
		await writeConfig(config, cwd);
	}
	const filePath = getExpertisePath(domain, cwd);
	if (!existsSync(filePath)) {
		await createExpertiseFile(filePath);
	}
}

export async function removeDomain(domain: string, cwd: string = process.cwd()): Promise<void> {
	validateDomainName(domain);
	const config = await readConfig(cwd);
	if (!(domain in config.domains)) {
		throw new Error(`Domain "${domain}" not found in config.`);
	}
	delete config.domains[domain];
	await writeConfig(config, cwd);
	const filePath = getExpertisePath(domain, cwd);
	if (existsSync(filePath)) {
		await rm(filePath);
	}
}

export async function writeConfig(config: AtlasConfig, cwd: string = process.cwd()): Promise<void> {
	const configPath = getConfigPath(cwd);
	const content = yaml.dump(config, { lineWidth: -1 });
	await writeFile(configPath, content, "utf-8");
}

export async function initAtlasDir(cwd: string = process.cwd()): Promise<void> {
	const atlasDir = getAtlasDir(cwd);
	const expertiseDir = getExpertiseDir(cwd);
	await mkdir(atlasDir, { recursive: true });
	await mkdir(expertiseDir, { recursive: true });

	// Only write default config if none exists — preserve user customizations.
	// Use the templated YAML (with commented-out optional knobs) for discoverability;
	// subsequent writes via writeConfig() round-trip through yaml.dump and lose comments.
	const configPath = getConfigPath(cwd);
	if (!existsSync(configPath)) {
		await writeFile(configPath, buildInitialConfigYaml(), "utf-8");
	}

	// Create or append .gitattributes with merge=union for JSONL files
	const gitattributesPath = join(cwd, ".gitattributes");
	let existing = "";
	try {
		existing = await readFile(gitattributesPath, "utf-8");
	} catch {
		// File doesn't exist yet — will create it
	}
	if (!existing.includes(GITATTRIBUTES_LINE)) {
		const separator = existing.length > 0 && !existing.endsWith("\n") ? "\n" : "";
		await writeFile(gitattributesPath, `${existing + separator + GITATTRIBUTES_LINE}\n`, "utf-8");
	}

	// Create .luuna/atlas/README.md if missing
	const readmePath = join(atlasDir, "README.md");
	if (!existsSync(readmePath)) {
		await writeFile(readmePath, ATLAS_README, "utf-8");
	}
}
