/** Exact readonly schema shapes exposed by the native compatibility transport. */
export type ConfigSchema = {
	readonly $schema: "http://json-schema.org/draft-07/schema#";
	readonly title: "Atlas Configuration";
	readonly description: "Schema for .luuna/atlas/config.yaml. Mirrors the AtlasConfig TypeScript interface in src/schemas/config.ts. Consumers (e.g. the Luuna desktop) render generic configuration UIs against this schema via `luuna-atlas config schema --json`.";
	readonly type: "object";
	readonly required: readonly ["version", "domains", "governance", "classification_defaults"];
	readonly properties: {
		readonly version: {
			readonly type: "string";
			readonly title: "Config version";
			readonly description: "Schema version of the on-disk config. Bumped only when the shape changes in a backwards-incompatible way.";
			readonly default: "1";
		};
		readonly domains: {
			readonly type: "object";
			readonly title: "Domains";
			readonly description: "Per-domain configuration keyed by domain name. Empty object is valid (a fresh project starts with no declared domains; `luuna-atlas record` auto-creates them).";
			readonly additionalProperties: {
				readonly type: "object";
				readonly title: "Domain configuration";
				readonly description: "Per-domain rules. Empty/missing fields preserve back-compat (any registered type accepted, no extra required fields).";
				readonly properties: {
					readonly allowed_types: {
						readonly type: "array";
						readonly title: "Allowed record types";
						readonly description: "Gates `--type` on write for this domain. Empty/missing means all registered types are allowed. `disabled_types` wins over `allowed_types` on overlap so retiring a type does not hard-fail peers.";
						readonly items: { readonly type: "string" };
					};
					readonly required_fields: {
						readonly type: "array";
						readonly title: "Required top-level fields";
						readonly description: "Additional top-level fields every record in this domain must carry, on top of the per-type schema requirements.";
						readonly items: { readonly type: "string" };
					};
				};
				readonly additionalProperties: false;
			};
			// biome-ignore lint/complexity/noBannedTypes: Preserve the original public schema's inferred empty literal type.
			readonly default: {};
		};
		readonly governance: {
			readonly type: "object";
			readonly title: "Governance limits";
			readonly description: "Per-domain record-count thresholds. Atlas warns at `warn_entries` and refuses writes past `hard_limit` (configurable per project).";
			readonly properties: {
				readonly max_entries: {
					readonly type: "integer";
					readonly title: "Soft target";
					readonly description: "Soft target for records per domain. Status reports flag domains above this number; writes are not blocked.";
					readonly minimum: 1;
					readonly default: 100;
				};
				readonly warn_entries: {
					readonly type: "integer";
					readonly title: "Warn threshold";
					readonly description: "Threshold at which `luuna-atlas record` prints a 'consider compacting' warning.";
					readonly minimum: 1;
					readonly default: 150;
				};
				readonly hard_limit: {
					readonly type: "integer";
					readonly title: "Hard limit";
					readonly description: "Maximum records per domain. Writes past this number are refused.";
					readonly minimum: 1;
					readonly default: 200;
				};
			};
			readonly required: readonly ["max_entries", "warn_entries", "hard_limit"];
			readonly additionalProperties: false;
		};
		readonly classification_defaults: {
			readonly type: "object";
			readonly title: "Classification defaults";
			readonly description: "Shelf-life windows (in days) for time-bound record classifications.";
			readonly properties: {
				readonly shelf_life: {
					readonly type: "object";
					readonly title: "Shelf life (days)";
					readonly description: "How long records of each classification remain in primary expertise before `luuna-atlas prune` considers them stale. `foundational` records are permanent and are not listed here.";
					readonly properties: {
						readonly tactical: {
							readonly type: "integer";
							readonly title: "Tactical shelf life";
							readonly description: "Days a tactical record remains live before becoming prune-eligible.";
							readonly minimum: 1;
							readonly default: 14;
						};
						readonly observational: {
							readonly type: "integer";
							readonly title: "Observational shelf life";
							readonly description: "Days an observational record remains live before becoming prune-eligible.";
							readonly minimum: 1;
							readonly default: 30;
						};
					};
					readonly required: readonly ["tactical", "observational"];
					readonly additionalProperties: false;
				};
			};
			readonly required: readonly ["shelf_life"];
			readonly additionalProperties: false;
		};
		readonly prime: {
			readonly type: "object";
			readonly title: "Prime defaults";
			readonly description: "Defaults applied when `luuna-atlas prime` is invoked without overriding flags.";
			readonly properties: {
				readonly default_mode: {
					readonly type: "string";
					readonly title: "Default prime mode";
					readonly description: "`full` (the default) emits the budget-capped record index, failures first; `manifest` emits a quick reference + per-domain counts instead of records.";
					readonly enum: readonly ["manifest", "full"];
					readonly default: "full";
				};
				readonly tier_weights: {
					readonly type: "object";
					readonly title: "Trust-tier ranking weights";
					readonly description: "Per-record sort score = stars * `star` + classification weight. Override only the knobs you want to retune; unset fields keep their default. Higher scores surface first.";
					readonly properties: {
						readonly star: {
							readonly type: "number";
							readonly title: "Star weight";
							readonly description: "Multiplier applied to a record's confirmation-score (\u00e2\u02dc\u2026 count from outcomes).";
							readonly minimum: 0;
							readonly default: 100;
						};
						readonly foundational: {
							readonly type: "number";
							readonly title: "Foundational weight";
							readonly description: "Base score added to every foundational record.";
							readonly minimum: 0;
							readonly default: 50;
						};
						readonly tactical: {
							readonly type: "number";
							readonly title: "Tactical weight";
							readonly description: "Base score added to every tactical record.";
							readonly minimum: 0;
							readonly default: 20;
						};
						readonly observational: {
							readonly type: "number";
							readonly title: "Observational weight";
							readonly description: "Base score added to every observational record.";
							readonly minimum: 0;
							readonly default: 10;
						};
					};
					readonly additionalProperties: false;
				};
				readonly session_close: {
					readonly type: "object";
					readonly title: "Session-close footer";
					readonly description: "Footer appended after `luuna-atlas prime` (and the embedded snippet emitted by `luuna-atlas setup` / `luuna-atlas onboard`). `style` selects a built-in preset; `custom` overrides the preset with verbatim prose.";
					readonly properties: {
						readonly style: {
							readonly type: "string";
							readonly title: "Preset style";
							readonly description: "`directive` = numbered imperative + type glossary + anti-filler guardrails. `conditional` = the v0.10.0 'if you discovered\u00e2\u20ac\u00a6' reframe (default). `minimal` = one-line nudge. `none` = suppress the footer entirely.";
							readonly enum: readonly ["directive", "conditional", "minimal", "none"];
							readonly default: "conditional";
						};
						readonly custom: {
							readonly type: "string";
							readonly title: "Custom override";
							readonly description: "Verbatim prose to emit in place of the preset. Wins over `style` when both are set; the embedded variant (used by `luuna-atlas setup` / `luuna-atlas onboard`) is also overridden.";
						};
					};
					readonly additionalProperties: false;
				};
			};
			readonly additionalProperties: false;
		};
		readonly search: {
			readonly type: "object";
			readonly title: "Search tuning";
			readonly description: "Knobs that tune `luuna-atlas search` / `luuna-atlas rank` ranking.";
			readonly properties: {
				readonly boost_factor: {
					readonly type: "number";
					readonly title: "Confirmation boost factor";
					readonly description: "Multiplier applied to BM25 scores via `applyConfirmationBoost`. 0 disables the boost.";
					readonly minimum: 0;
					readonly default: 0.1;
				};
			};
			readonly required: readonly ["boost_factor"];
			readonly additionalProperties: false;
		};
		readonly custom_types: {
			readonly type: "object";
			readonly title: "Custom record types";
			readonly description: "Project-defined record types keyed by type name. Treated identically to built-ins by the registry (CLI flags, validation, dedup, formatters).";
			readonly additionalProperties: {
				readonly type: "object";
				readonly title: "Custom record type";
				readonly description: "Definition of a custom record type. May extend a built-in to inherit its required/optional fields, dedup_key, id_key, summary, compact strategy, section title, and file-extraction behavior \u00e2\u20ac\u201d override only what differs; arrays merge as a union. Custom-from-custom is not supported.";
				readonly properties: {
					readonly extends: {
						readonly type: "string";
						readonly title: "Built-in to extend";
						readonly description: "Name of a built-in type (convention, pattern, failure, decision, reference, guide). Custom-from-custom is rejected.";
						readonly enum: readonly [
							"convention",
							"pattern",
							"failure",
							"decision",
							"reference",
							"guide",
						];
					};
					readonly required: {
						readonly type: "array";
						readonly title: "Required fields";
						readonly description: "Top-level fields a record of this type must carry.";
						readonly items: { readonly type: "string" };
					};
					readonly optional: {
						readonly type: "array";
						readonly title: "Optional fields";
						readonly description: "Additional top-level fields a record of this type may carry.";
						readonly items: { readonly type: "string" };
					};
					readonly dedup_key: {
						readonly type: "string";
						readonly title: "Dedup key";
						readonly description: "Field used to detect duplicates on write (e.g. 'name' or 'content').";
					};
					readonly id_key: {
						readonly type: "string";
						readonly title: "Display ID field";
						readonly description: "Field used to render a record's identifying label in compact output.";
					};
					readonly summary: {
						readonly type: "string";
						readonly title: "Summary template";
						readonly description: "Compact-line summary template used by `luuna-atlas prime`.";
					};
					readonly extracts_files: {
						readonly type: "boolean";
						readonly title: "Extracts files";
						readonly description: "Whether records of this type carry a list of file anchors.";
					};
					readonly files_field: {
						readonly type: "string";
						readonly title: "Files field name";
						readonly description: "Field name that holds file anchors when extracts_files is true (e.g. 'files').";
					};
					readonly compact: {
						readonly type: "string";
						readonly title: "Compact strategy";
						readonly description: "How `luuna-atlas compact` collapses duplicates of this type.";
						readonly enum: readonly ["concat", "merge_outcomes", "keep_latest", "manual"];
					};
					readonly section_title: {
						readonly type: "string";
						readonly title: "Section title";
						readonly description: "Heading used when grouping this type in `luuna-atlas prime` output.";
					};
					readonly aliases: {
						readonly type: "object";
						readonly title: "Field aliases";
						readonly description: "Map canonical (current) field name to legacy aliases. Records on disk that carry a legacy field are rewritten to the canonical name at read time.";
						readonly additionalProperties: {
							readonly type: "array";
							readonly items: { readonly type: "string" };
						};
					};
				};
				readonly additionalProperties: false;
			};
		};
		readonly decay: {
			readonly type: "object";
			readonly title: "Decay knobs";
			readonly description: "Configuration for tier-demotion behavior in `luuna-atlas prune`. Currently only `anchor_validity` is wired; `weight` is reserved for the future fitness blend.";
			readonly properties: {
				readonly anchor_validity: {
					readonly type: "object";
					readonly title: "Anchor-validity decay";
					readonly description: "Tier-demotion knobs for `luuna-atlas prune --check-anchors`. A record decays one tier per pass when its file/dir anchors stop resolving and `valid_fraction` drops below `threshold`, provided the record is older than `grace_days`. Records with zero anchors are exempt (absence means 'applies globally').";
					readonly properties: {
						readonly threshold: {
							readonly type: "number";
							readonly title: "Validity threshold";
							readonly description: "Demote when the fraction of resolving anchors drops below this value. Range: [0, 1].";
							readonly minimum: 0;
							readonly maximum: 1;
							readonly default: 0.5;
						};
						readonly grace_days: {
							readonly type: "integer";
							readonly title: "Grace period (days)";
							readonly description: "Records younger than this many days are exempt from anchor-validity decay so brand-new records can stabilize.";
							readonly minimum: 0;
							readonly default: 7;
						};
						readonly weight: {
							readonly type: "number";
							readonly title: "Fitness weight";
							readonly description: "Reserved for the future fitness blend (R-05g). Currently unused; safe to leave unset.";
						};
					};
					readonly additionalProperties: false;
				};
			};
			readonly additionalProperties: false;
		};
		readonly audit: {
			readonly type: "object";
			readonly title: "Audit configuration";
			readonly description: "Knobs for `luuna-atlas audit`. Global thresholds are merged on top of defaults; per-domain entries layer on top of the global thresholds for that domain only.";
			readonly properties: {
				readonly thresholds: {
					readonly type: "object";
					readonly title: "Audit thresholds";
					readonly description: "PASS/WARN/FAIL bands for `luuna-atlas audit`. Below `_warn` is FAIL; between `_warn` and the primary threshold is WARN; meeting the primary threshold is PASS. Floater and stale knobs use a single PASS ceiling.";
					readonly properties: {
						readonly evidence_coverage: {
							readonly type: "number";
							readonly title: "Evidence coverage (PASS)";
							readonly description: "PASS when \u00e2\u2030\u00a5 this fraction of records carry any tracker (missions/gh/linear/bead) or commit evidence. Range: [0, 1].";
							readonly minimum: 0;
							readonly maximum: 1;
							readonly default: 0.5;
						};
						readonly evidence_coverage_warn: {
							readonly type: "number";
							readonly title: "Evidence coverage (WARN)";
							readonly description: "WARN when evidence coverage is between this value and `evidence_coverage`. Below this is FAIL. Range: [0, 1].";
							readonly minimum: 0;
							readonly maximum: 1;
							readonly default: 0.3;
						};
						readonly floater_max: {
							readonly type: "number";
							readonly title: "Floater rate (PASS ceiling)";
							readonly description: "PASS when the fraction of records without any tracker, relates_to, or commit evidence is \u00e2\u2030\u00a4 this value. Above is WARN/FAIL.";
							readonly minimum: 0;
							readonly maximum: 1;
							readonly default: 0.2;
						};
						readonly rule_density_min: {
							readonly type: "number";
							readonly title: "Convention rule-density (PASS)";
							readonly description: "PASS when \u00e2\u2030\u00a5 this fraction of convention records contain a rule-signal word (because, must not, avoid, always, never, \u00e2\u20ac\u00a6). Range: [0, 1].";
							readonly minimum: 0;
							readonly maximum: 1;
							readonly default: 0.25;
						};
						readonly rule_density_warn: {
							readonly type: "number";
							readonly title: "Convention rule-density (WARN)";
							readonly description: "WARN when rule-density is between this value and `rule_density_min`. Below this is FAIL. Range: [0, 1].";
							readonly minimum: 0;
							readonly maximum: 1;
							readonly default: 0.15;
						};
						readonly max_records_per_domain: {
							readonly type: "integer";
							readonly title: "Records per domain (PASS ceiling)";
							readonly description: "Informational ceiling on records per domain; auditors flag domains above this number. Distinct from `governance.hard_limit` which gates writes.";
							readonly minimum: 1;
							readonly default: 200;
						};
						readonly max_stale: {
							readonly type: "integer";
							readonly title: "Stale records (PASS ceiling)";
							readonly description: "Informational ceiling on stale (past-shelf-life) records per domain. PASS when \u00e2\u2030\u00a4 this value.";
							readonly minimum: 0;
							readonly default: 0;
						};
					};
					readonly additionalProperties: false;
				};
				readonly ignore_domains: {
					readonly type: "array";
					readonly title: "Domains excluded from audit";
					readonly description: "Domains skipped entirely by `luuna-atlas audit` (e.g. reference-doc domains that are legitimately convention-heavy and evidence-light).";
					readonly items: { readonly type: "string" };
				};
				readonly per_domain: {
					readonly type: "object";
					readonly title: "Per-domain threshold overrides";
					readonly description: "Threshold overrides keyed by domain name. Partial overrides are merged on top of the global thresholds; keys not declared inherit the global value.";
					readonly additionalProperties: {
						readonly type: "object";
						readonly title: "Audit thresholds";
						readonly description: "PASS/WARN/FAIL bands for `luuna-atlas audit`. Below `_warn` is FAIL; between `_warn` and the primary threshold is WARN; meeting the primary threshold is PASS. Floater and stale knobs use a single PASS ceiling.";
						readonly properties: {
							readonly evidence_coverage: {
								readonly type: "number";
								readonly title: "Evidence coverage (PASS)";
								readonly description: "PASS when \u00e2\u2030\u00a5 this fraction of records carry any tracker (missions/gh/linear/bead) or commit evidence. Range: [0, 1].";
								readonly minimum: 0;
								readonly maximum: 1;
								readonly default: 0.5;
							};
							readonly evidence_coverage_warn: {
								readonly type: "number";
								readonly title: "Evidence coverage (WARN)";
								readonly description: "WARN when evidence coverage is between this value and `evidence_coverage`. Below this is FAIL. Range: [0, 1].";
								readonly minimum: 0;
								readonly maximum: 1;
								readonly default: 0.3;
							};
							readonly floater_max: {
								readonly type: "number";
								readonly title: "Floater rate (PASS ceiling)";
								readonly description: "PASS when the fraction of records without any tracker, relates_to, or commit evidence is \u00e2\u2030\u00a4 this value. Above is WARN/FAIL.";
								readonly minimum: 0;
								readonly maximum: 1;
								readonly default: 0.2;
							};
							readonly rule_density_min: {
								readonly type: "number";
								readonly title: "Convention rule-density (PASS)";
								readonly description: "PASS when \u00e2\u2030\u00a5 this fraction of convention records contain a rule-signal word (because, must not, avoid, always, never, \u00e2\u20ac\u00a6). Range: [0, 1].";
								readonly minimum: 0;
								readonly maximum: 1;
								readonly default: 0.25;
							};
							readonly rule_density_warn: {
								readonly type: "number";
								readonly title: "Convention rule-density (WARN)";
								readonly description: "WARN when rule-density is between this value and `rule_density_min`. Below this is FAIL. Range: [0, 1].";
								readonly minimum: 0;
								readonly maximum: 1;
								readonly default: 0.15;
							};
							readonly max_records_per_domain: {
								readonly type: "integer";
								readonly title: "Records per domain (PASS ceiling)";
								readonly description: "Informational ceiling on records per domain; auditors flag domains above this number. Distinct from `governance.hard_limit` which gates writes.";
								readonly minimum: 1;
								readonly default: 200;
							};
							readonly max_stale: {
								readonly type: "integer";
								readonly title: "Stale records (PASS ceiling)";
								readonly description: "Informational ceiling on stale (past-shelf-life) records per domain. PASS when \u00e2\u2030\u00a4 this value.";
								readonly minimum: 0;
								readonly default: 0;
							};
						};
						readonly additionalProperties: false;
					};
				};
			};
			readonly additionalProperties: false;
		};
		readonly quality: {
			readonly type: "object";
			readonly title: "Record quality gate";
			readonly description: "Write-time checks on `luuna-atlas record`: near-duplicates, conventions/patterns with no anchor or evidence, conventions that restate code. `level`: `warn` (default) prints and writes, `error` blocks unless --force, `off` skips. Exact duplicates are always blocked.";
			readonly properties: {
				readonly level: {
					readonly type: "string";
					readonly enum: readonly ["warn", "error", "off"];
					readonly default: "warn";
				};
			};
			readonly additionalProperties: false;
		};
		readonly disabled_types: {
			readonly type: "array";
			readonly title: "Disabled types";
			readonly description: "Names of registered types (built-in or custom) that emit a deprecation warning on write. Reads still succeed; the type stays in CLI choices.";
			readonly items: { readonly type: "string" };
		};
		readonly hooks: {
			readonly type: "object";
			readonly title: "Lifecycle hooks";
			readonly description: "Ordered shell scripts invoked at each lifecycle event. Scripts receive the payload as JSON on stdin (with LUUNA_ATLAS_HOOK=1; cwd at project root). `pre-*` hooks block on non-zero exit; `post-*` hooks emit a warning. `pre-record`, `pre-prime`, and `pre-compact` may mutate the payload via stdout JSON; `pre-prune` is block-or-allow only.";
			readonly properties: {
				readonly "pre-record": {
					readonly type: "array";
					readonly description: "Ordered shell commands invoked for this lifecycle event.";
					readonly items: { readonly type: "string" };
				};
				readonly "post-record": {
					readonly type: "array";
					readonly description: "Ordered shell commands invoked for this lifecycle event.";
					readonly items: { readonly type: "string" };
				};
				readonly "pre-prime": {
					readonly type: "array";
					readonly description: "Ordered shell commands invoked for this lifecycle event.";
					readonly items: { readonly type: "string" };
				};
				readonly "pre-prune": {
					readonly type: "array";
					readonly description: "Ordered shell commands invoked for this lifecycle event.";
					readonly items: { readonly type: "string" };
				};
				readonly "pre-compact": {
					readonly type: "array";
					readonly description: "Ordered shell commands invoked for this lifecycle event.";
					readonly items: { readonly type: "string" };
				};
			};
			readonly additionalProperties: false;
		};
		readonly hook_settings: {
			readonly type: "object";
			readonly title: "Hook execution settings";
			readonly description: "Per-hook execution settings.";
			readonly properties: {
				readonly timeout_ms: {
					readonly type: "integer";
					readonly title: "Hook timeout (ms)";
					readonly description: "Maximum runtime per hook script. SIGKILL on timeout. Default is 5000ms.";
					readonly minimum: 1;
					readonly default: 5000;
				};
			};
			readonly additionalProperties: false;
		};
	};
	readonly additionalProperties: false;
};
export type RecordSchema = {
	readonly $schema: "http://json-schema.org/draft-07/schema#";
	readonly title: "Atlas Expertise Record";
	readonly description: "A single expertise record in a Atlas domain file";
	readonly type: "object";
	readonly definitions: {
		readonly classification: {
			readonly type: "string";
			readonly enum: readonly ["foundational", "tactical", "observational"];
		};
		readonly evidence: {
			readonly type: "object";
			readonly properties: {
				readonly commit: { readonly type: "string" };
				readonly date: { readonly type: "string" };
				readonly issue: { readonly type: "string" };
				readonly file: { readonly type: "string" };
				readonly bead: { readonly type: "string" };
				readonly missions: { readonly type: "string" };
				readonly seeds: { readonly type: "string" };
				readonly gh: { readonly type: "string" };
				readonly linear: { readonly type: "string" };
			};
			readonly additionalProperties: false;
		};
		readonly outcome: {
			readonly type: "object";
			readonly properties: {
				readonly status: {
					readonly type: "string";
					readonly enum: readonly ["success", "failure", "partial"];
				};
				readonly duration: { readonly type: "number" };
				readonly test_results: { readonly type: "string" };
				readonly agent: { readonly type: "string" };
				readonly notes: { readonly type: "string" };
				readonly recorded_at: { readonly type: "string" };
			};
			readonly required: readonly ["status"];
			readonly additionalProperties: false;
		};
	};
	readonly oneOf: readonly [
		{
			readonly type: "object";
			readonly properties: {
				readonly id: { readonly type: "string"; readonly pattern: "^mx-[0-9a-f]{4,8}$" };
				readonly type: { readonly type: "string"; readonly const: "convention" };
				readonly content: { readonly type: "string" };
				readonly classification: { readonly $ref: "#/definitions/classification" };
				readonly recorded_at: { readonly type: "string" };
				readonly evidence: { readonly $ref: "#/definitions/evidence" };
				readonly tags: { readonly type: "array"; readonly items: { readonly type: "string" } };
				readonly relates_to: {
					readonly type: "array";
					readonly items: {
						readonly type: "string";
						readonly pattern: "^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$";
					};
				};
				readonly supersedes: {
					readonly type: "array";
					readonly items: {
						readonly type: "string";
						readonly pattern: "^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$";
					};
				};
				readonly outcomes: {
					readonly type: "array";
					readonly items: { readonly $ref: "#/definitions/outcome" };
				};
				readonly dir_anchors: {
					readonly type: "array";
					readonly items: { readonly type: "string" };
				};
				readonly supersession_demoted_at: { readonly type: "string" };
				readonly anchor_decay_demoted_at: { readonly type: "string" };
				readonly owner: { readonly type: "string" };
				readonly status: {
					readonly type: "string";
					readonly enum: readonly ["draft", "active", "deprecated"];
				};
			};
			readonly required: readonly ["type", "content", "classification", "recorded_at"];
			readonly additionalProperties: false;
		},
		{
			readonly type: "object";
			readonly properties: {
				readonly id: { readonly type: "string"; readonly pattern: "^mx-[0-9a-f]{4,8}$" };
				readonly type: { readonly type: "string"; readonly const: "pattern" };
				readonly name: { readonly type: "string" };
				readonly description: { readonly type: "string" };
				readonly files: { readonly type: "array"; readonly items: { readonly type: "string" } };
				readonly classification: { readonly $ref: "#/definitions/classification" };
				readonly recorded_at: { readonly type: "string" };
				readonly evidence: { readonly $ref: "#/definitions/evidence" };
				readonly tags: { readonly type: "array"; readonly items: { readonly type: "string" } };
				readonly relates_to: {
					readonly type: "array";
					readonly items: {
						readonly type: "string";
						readonly pattern: "^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$";
					};
				};
				readonly supersedes: {
					readonly type: "array";
					readonly items: {
						readonly type: "string";
						readonly pattern: "^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$";
					};
				};
				readonly outcomes: {
					readonly type: "array";
					readonly items: { readonly $ref: "#/definitions/outcome" };
				};
				readonly dir_anchors: {
					readonly type: "array";
					readonly items: { readonly type: "string" };
				};
				readonly supersession_demoted_at: { readonly type: "string" };
				readonly anchor_decay_demoted_at: { readonly type: "string" };
				readonly owner: { readonly type: "string" };
				readonly status: {
					readonly type: "string";
					readonly enum: readonly ["draft", "active", "deprecated"];
				};
			};
			readonly required: readonly ["type", "name", "description", "classification", "recorded_at"];
			readonly additionalProperties: false;
		},
		{
			readonly type: "object";
			readonly properties: {
				readonly id: { readonly type: "string"; readonly pattern: "^mx-[0-9a-f]{4,8}$" };
				readonly type: { readonly type: "string"; readonly const: "failure" };
				readonly description: { readonly type: "string" };
				readonly resolution: { readonly type: "string" };
				readonly classification: { readonly $ref: "#/definitions/classification" };
				readonly recorded_at: { readonly type: "string" };
				readonly evidence: { readonly $ref: "#/definitions/evidence" };
				readonly tags: { readonly type: "array"; readonly items: { readonly type: "string" } };
				readonly relates_to: {
					readonly type: "array";
					readonly items: {
						readonly type: "string";
						readonly pattern: "^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$";
					};
				};
				readonly supersedes: {
					readonly type: "array";
					readonly items: {
						readonly type: "string";
						readonly pattern: "^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$";
					};
				};
				readonly outcomes: {
					readonly type: "array";
					readonly items: { readonly $ref: "#/definitions/outcome" };
				};
				readonly dir_anchors: {
					readonly type: "array";
					readonly items: { readonly type: "string" };
				};
				readonly supersession_demoted_at: { readonly type: "string" };
				readonly anchor_decay_demoted_at: { readonly type: "string" };
				readonly owner: { readonly type: "string" };
				readonly status: {
					readonly type: "string";
					readonly enum: readonly ["draft", "active", "deprecated"];
				};
			};
			readonly required: readonly [
				"type",
				"description",
				"resolution",
				"classification",
				"recorded_at",
			];
			readonly additionalProperties: false;
		},
		{
			readonly type: "object";
			readonly properties: {
				readonly id: { readonly type: "string"; readonly pattern: "^mx-[0-9a-f]{4,8}$" };
				readonly type: { readonly type: "string"; readonly const: "decision" };
				readonly title: { readonly type: "string" };
				readonly rationale: { readonly type: "string" };
				readonly date: { readonly type: "string" };
				readonly classification: { readonly $ref: "#/definitions/classification" };
				readonly recorded_at: { readonly type: "string" };
				readonly evidence: { readonly $ref: "#/definitions/evidence" };
				readonly tags: { readonly type: "array"; readonly items: { readonly type: "string" } };
				readonly relates_to: {
					readonly type: "array";
					readonly items: {
						readonly type: "string";
						readonly pattern: "^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$";
					};
				};
				readonly supersedes: {
					readonly type: "array";
					readonly items: {
						readonly type: "string";
						readonly pattern: "^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$";
					};
				};
				readonly outcomes: {
					readonly type: "array";
					readonly items: { readonly $ref: "#/definitions/outcome" };
				};
				readonly dir_anchors: {
					readonly type: "array";
					readonly items: { readonly type: "string" };
				};
				readonly supersession_demoted_at: { readonly type: "string" };
				readonly anchor_decay_demoted_at: { readonly type: "string" };
				readonly owner: { readonly type: "string" };
				readonly status: {
					readonly type: "string";
					readonly enum: readonly ["draft", "active", "deprecated"];
				};
			};
			readonly required: readonly ["type", "title", "rationale", "classification", "recorded_at"];
			readonly additionalProperties: false;
		},
		{
			readonly type: "object";
			readonly properties: {
				readonly id: { readonly type: "string"; readonly pattern: "^mx-[0-9a-f]{4,8}$" };
				readonly type: { readonly type: "string"; readonly const: "reference" };
				readonly name: { readonly type: "string" };
				readonly description: { readonly type: "string" };
				readonly files: { readonly type: "array"; readonly items: { readonly type: "string" } };
				readonly classification: { readonly $ref: "#/definitions/classification" };
				readonly recorded_at: { readonly type: "string" };
				readonly evidence: { readonly $ref: "#/definitions/evidence" };
				readonly tags: { readonly type: "array"; readonly items: { readonly type: "string" } };
				readonly relates_to: {
					readonly type: "array";
					readonly items: {
						readonly type: "string";
						readonly pattern: "^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$";
					};
				};
				readonly supersedes: {
					readonly type: "array";
					readonly items: {
						readonly type: "string";
						readonly pattern: "^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$";
					};
				};
				readonly outcomes: {
					readonly type: "array";
					readonly items: { readonly $ref: "#/definitions/outcome" };
				};
				readonly dir_anchors: {
					readonly type: "array";
					readonly items: { readonly type: "string" };
				};
				readonly supersession_demoted_at: { readonly type: "string" };
				readonly anchor_decay_demoted_at: { readonly type: "string" };
				readonly owner: { readonly type: "string" };
				readonly status: {
					readonly type: "string";
					readonly enum: readonly ["draft", "active", "deprecated"];
				};
			};
			readonly required: readonly ["type", "name", "description", "classification", "recorded_at"];
			readonly additionalProperties: false;
		},
		{
			readonly type: "object";
			readonly properties: {
				readonly id: { readonly type: "string"; readonly pattern: "^mx-[0-9a-f]{4,8}$" };
				readonly type: { readonly type: "string"; readonly const: "guide" };
				readonly name: { readonly type: "string" };
				readonly description: { readonly type: "string" };
				readonly classification: { readonly $ref: "#/definitions/classification" };
				readonly recorded_at: { readonly type: "string" };
				readonly evidence: { readonly $ref: "#/definitions/evidence" };
				readonly tags: { readonly type: "array"; readonly items: { readonly type: "string" } };
				readonly relates_to: {
					readonly type: "array";
					readonly items: {
						readonly type: "string";
						readonly pattern: "^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$";
					};
				};
				readonly supersedes: {
					readonly type: "array";
					readonly items: {
						readonly type: "string";
						readonly pattern: "^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$";
					};
				};
				readonly outcomes: {
					readonly type: "array";
					readonly items: { readonly $ref: "#/definitions/outcome" };
				};
				readonly dir_anchors: {
					readonly type: "array";
					readonly items: { readonly type: "string" };
				};
				readonly supersession_demoted_at: { readonly type: "string" };
				readonly anchor_decay_demoted_at: { readonly type: "string" };
				readonly owner: { readonly type: "string" };
				readonly status: {
					readonly type: "string";
					readonly enum: readonly ["draft", "active", "deprecated"];
				};
			};
			readonly required: readonly ["type", "name", "description", "classification", "recorded_at"];
			readonly additionalProperties: false;
		},
	];
};
