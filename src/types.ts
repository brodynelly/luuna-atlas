/** Type-only compatibility contract; runtime behavior is implemented in Go. */
export type BuiltinRecordType =
	| "convention"
	| "pattern"
	| "failure"
	| "decision"
	| "reference"
	| "guide";

// Phase 2: widened to string so config-declared custom types are first-class.
// Use `BuiltinRecordType` for narrowing back to the six known shapes (e.g.,
// `registry.get(r.type)?.kind === "builtin"` then cast to a builtin record).
export type RecordType = string;

export type Classification = "foundational" | "tactical" | "observational";

export interface Evidence {
	commit?: string;
	date?: string;
	issue?: string;
	file?: string;
	bead?: string;
	missions?: string;
	gh?: string;
	linear?: string;
}

export interface Outcome {
	status: "success" | "failure" | "partial";
	duration?: number;
	test_results?: string;
	agent?: string;
	notes?: string;
	recorded_at?: string;
}

interface BaseRecord {
	id?: string;
	classification: Classification;
	recorded_at: string;
	evidence?: Evidence;
	tags?: string[];
	relates_to?: string[];
	supersedes?: string[];
	outcomes?: Outcome[];
	dir_anchors?: string[];
	// R-06 ownership: opaque handle (e.g. "@user" or "@team"). Stamped by the
	// owner-resolution chain at write time (atlas-41e1) and consumed by
	// `luuna-atlas review` (atlas-8cce). No internal validation in v1.
	owner?: string;
	// Lifecycle status. "draft" / "active" / "deprecated" are author-set on
	// live records; "archived" is set by `luuna-atlas prune` soft-archive (lives only
	// under .luuna/atlas/archive/<domain>.jsonl) and the live AJV schemas reject it.
	status?: "draft" | "active" | "deprecated" | "archived";
	archived_at?: string;
	// Set by `archiveRecords` on every code path that moves a record to
	// .luuna/atlas/archive/. Convention values: "stale" | "superseded" |
	// "anchor_decay" | "manual" | "compacted"; a free-text suffix is permitted
	// (e.g. "manual: wrong domain"). Optional for back-compat with archives
	// written before atlas-b41a landed; lives only on archived records.
	archive_reason?: string;
	// Set by `luuna-atlas prune` when supersession decay (R-05e) demotes a record one
	// classification tier. Lives on the record across the demotion until
	// archive; bumped each pass that re-demotes.
	supersession_demoted_at?: string;
	// Set by `luuna-atlas prune --check-anchors` when anchor-validity decay (R-05f)
	// demotes a record one tier because too many of its file/dir anchors no
	// longer resolve. Bumped each pass that re-demotes.
	anchor_decay_demoted_at?: string;
}

export interface ConventionRecord extends BaseRecord {
	type: "convention";
	content: string;
}

export interface PatternRecord extends BaseRecord {
	type: "pattern";
	name: string;
	description: string;
	files?: string[];
}

export interface FailureRecord extends BaseRecord {
	type: "failure";
	description: string;
	resolution: string;
}

export interface DecisionRecord extends BaseRecord {
	type: "decision";
	title: string;
	rationale: string;
	date?: string;
}

export interface ReferenceRecord extends BaseRecord {
	type: "reference";
	name: string;
	description: string;
	files?: string[];
}

export interface GuideRecord extends BaseRecord {
	type: "guide";
	name: string;
	description: string;
}

export type ExpertiseRecord =
	| ConventionRecord
	| PatternRecord
	| FailureRecord
	| DecisionRecord
	| ReferenceRecord
	| GuideRecord;

export type PrimeMode = "manifest" | "full";

// Session-close footer styles. Audit (V1_PLAN §3) found 70-80% of conventions
// were ritual restatements driven by an earlier "you MUST run this checklist"
// prose; the v0.10.0 reframing dropped it for `conditional`. `directive` ships
// a numbered imperative + type glossary + anti-filler guardrails for projects
// that need a louder voice. `minimal` is a one-line nudge. `none` suppresses
// the footer entirely. `custom` (via SessionCloseConfig.custom) wins over
// style and ships the raw string verbatim.
export type SessionCloseStyleName = "directive" | "conditional" | "minimal" | "none";

export interface SessionCloseConfig {
	style?: SessionCloseStyleName;
	custom?: string;
}

// Trust-tier ranking weights for `luuna-atlas prime` full-mode output (v0.10 slice 3).
// Sort score = stars * star_weight + classification_weight. Higher scores are
// surfaced first; ties preserve insertion order so within-tier output stays
// stable across runs.
export interface PrimeTierWeights {
	star?: number;
	foundational?: number;
	tactical?: number;
	observational?: number;
}

// Lifecycle events. `pre-*` hooks block on non-zero exit. `pre-record`,
// `pre-prime`, and `pre-compact` may mutate the payload via stdout JSON;
// `pre-prune` is block-or-allow only (its stdout is ignored, so a hook cannot
// reshape the candidate set). `post-*` hooks are observation-only: a non-zero
// exit emits a warning but never fails the parent command.
export type HookEvent = "pre-record" | "post-record" | "pre-prime" | "pre-prune" | "pre-compact";

export type HooksConfig = Partial<Record<HookEvent, string[]>>;

export type CustomCompactStrategy = "concat" | "merge_outcomes" | "keep_latest" | "manual";

export interface CustomTypeConfig {
	// Inherit required/optional/dedup_key/id_key/summary/compact/section_title/
	// extracts_files/files_field from a built-in type. Custom-from-custom is not
	// supported in v1. When set, all other fields override only what differs;
	// arrays merge as a union.
	extends?: string;
	required?: string[];
	optional?: string[];
	dedup_key?: string;
	id_key?: string;
	summary?: string;
	extracts_files?: boolean;
	files_field?: string;
	compact?: CustomCompactStrategy;
	section_title?: string;
	// Map canonical (current) field name to legacy aliases. Records on disk that
	// carry a legacy field are rewritten to the canonical name at read time.
	aliases?: Record<string, string[]>;
}

// Per-domain configuration. `allowed_types` gates which registered record
// types may be written into the domain — empty/missing means all registered
// types are allowed (back-compat). `required_fields` lists additional
// top-level fields a record must carry on top of its per-type requirements;
// empty/missing means no extra requirements.
export interface DomainConfig {
	allowed_types?: string[];
	required_fields?: string[];
}

export interface AnchorValidityConfig {
	threshold?: number;
	grace_days?: number;
	weight?: number;
}

export interface DecayConfig {
	anchor_validity?: AnchorValidityConfig;
}

// `luuna-atlas audit` thresholds. PASS/WARN/FAIL bands for the three primary corpus
// health metrics. Defaults relaxed from the Python prototype per V1_PLAN §4.2
// ("0.7 evidence coverage is empirically unreachable today"). All knobs are
// overridable globally under `audit.thresholds` and per-domain under
// `audit.per_domain.<name>` — partial overrides are merged on top of defaults.
export interface AuditThresholds {
	evidence_coverage?: number; // PASS ≥, default 0.5
	evidence_coverage_warn?: number; // WARN ≥, default 0.3 (below = FAIL)
	floater_max?: number; // PASS ≤ (fraction of records), default 0.2
	rule_density_min?: number; // PASS ≥, default 0.25
	rule_density_warn?: number; // WARN ≥, default 0.15 (below = FAIL)
	max_records_per_domain?: number; // PASS ≤, default 200 (matches governance.hard_limit)
	max_stale?: number; // PASS ≤ stale-record count, default 0
}

export interface AuditConfig {
	thresholds?: AuditThresholds;
	// Domains excluded from audit metrics entirely (e.g. reference-doc domains
	// that are legitimately convention-heavy and evidence-light).
	ignore_domains?: string[];
	// Per-domain threshold overrides. Each value is merged on top of the global
	// thresholds for that domain only — keys not declared fall back to the global.
	per_domain?: Record<string, AuditThresholds>;
}

// Write-time quality gate for `luuna-atlas record` (near-duplicates, ungrounded
// conventions/patterns, code-restatement conventions). `warn` prints and
// writes; `error` blocks unless --force; `off` skips the checks. Exact
// duplicates are blocked regardless of level.
export type QualityLevel = "warn" | "error" | "off";

export interface QualityConfig {
	level?: QualityLevel;
}

export interface AtlasConfig {
	version: string;
	domains: Record<string, DomainConfig>;
	governance: {
		max_entries: number;
		warn_entries: number;
		hard_limit: number;
	};
	classification_defaults: {
		shelf_life: {
			tactical: number;
			observational: number;
		};
	};
	prime?: {
		default_mode?: PrimeMode;
		// Trust-tier ranking weights. Each knob is optional; unset fields fall
		// back to DEFAULT_PRIME_TIER_WEIGHTS so projects can tune one dimension
		// (e.g. observational only) without redeclaring the whole block.
		tier_weights?: PrimeTierWeights;
		// Session-close footer customization. `style` selects a built-in preset
		// (directive / conditional / minimal / none); `custom` overrides the
		// preset entirely with verbatim prose. Both are optional — unset means
		// DEFAULT_SESSION_CLOSE_STYLE ("conditional", v0.10.0 back-compat).
		session_close?: SessionCloseConfig;
	};
	search?: {
		// Multiplier applied to BM25 scores via applyConfirmationBoost. 0 disables.
		boost_factor: number;
	};
	custom_types?: Record<string, CustomTypeConfig>;
	// Decay knobs. R-05f wires `anchor_validity` into `luuna-atlas prune --check-anchors`;
	// `weight` is reserved for the future R-05g fitness blend and otherwise unused.
	decay?: DecayConfig;
	// `luuna-atlas audit` thresholds and ignore-list. See AuditConfig.
	audit?: AuditConfig;
	// `luuna-atlas record` write-time quality gate. See QualityConfig.
	quality?: QualityConfig;
	// Names of registered types (built-in or custom) that emit a deprecation
	// warning on write. Reads still succeed; the type stays in CLI choices.
	disabled_types?: string[];
	// Lifecycle hook scripts. Each event maps to an ordered array of shell
	// commands. Atlas invokes each script with the relevant payload as JSON on
	// stdin. See HookEvent for semantics.
	hooks?: HooksConfig;
	// Per-hook execution settings. `timeout_ms` defaults to DEFAULT_HOOK_TIMEOUT_MS.
	hook_settings?: {
		timeout_ms?: number;
	};
}

export type ScoredRecord = ExpertiseRecord;
