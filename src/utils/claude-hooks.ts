// `luuna-atlas setup claude`: the luuna-atlas handlers in <project>/.claude/settings.json.
//
//   SessionStart  luuna-atlas prime   compact record index at startup/resume/compact
//   PreToolUse    luuna-atlas hook    Read|Edit|Write|MultiEdit|NotebookEdit|Bash:
//                            inject records anchored to the file, deny hand
//                            edits of .luuna/atlas/expertise/*.jsonl
//
// Claude Code nests handlers in matcher groups:
//   {"hooks": {"<Event>": [{"matcher": "...", "hooks": [{"type": "command", "command": "..."}]}]}}
// Atlas owns exactly the handlers whose command is `luuna-atlas prime|hook` (or the
// pre-Luuna command names, which are upgraded in place);
// install strips those and appends one fresh group per event, leaving every
// other hook as it was. Re-running is a no-op. Remove strips luuna-atlas handlers
// from every event (including the legacy PreCompact entry) and drops groups
// and events they leave empty. Mirrors `luuna-missions setup claude`.

import { LEGACY_HOOK_COMMAND_RE } from "./legacy-layout.ts";

type Json = Record<string, unknown>;

interface HookSpec {
	event: "SessionStart" | "PreToolUse";
	matcher: string;
	command: string;
}

export const CLAUDE_HOOKS: readonly HookSpec[] = [
	{ event: "SessionStart", matcher: "", command: "luuna-atlas prime" },
	{
		event: "PreToolUse",
		matcher: "Read|Edit|Write|MultiEdit|NotebookEdit|Bash",
		command: "luuna-atlas hook",
	},
];

const ATLAS_COMMAND_RE = /^\s*luuna-atlas\s+(prime|hook)(\s|$)/;

function isObject(v: unknown): v is Json {
	return typeof v === "object" && v !== null && !Array.isArray(v);
}

function isAtlasHandler(h: unknown): boolean {
	return (
		isObject(h) &&
		typeof h.command === "string" &&
		(ATLAS_COMMAND_RE.test(h.command) || LEGACY_HOOK_COMMAND_RE.test(h.command))
	);
}

function groupHooks(g: unknown): unknown[] {
	return isObject(g) && Array.isArray(g.hooks) ? g.hooks : [];
}

function makeGroup(spec: HookSpec): Json {
	return { matcher: spec.matcher, hooks: [{ type: "command", command: spec.command }] };
}

function stripAtlas(groups: readonly unknown[]): unknown[] {
	const out: unknown[] = [];
	for (const g of groups) {
		const hooks = groupHooks(g);
		const kept = hooks.filter((h) => !isAtlasHandler(h));
		if (kept.length === hooks.length) out.push(g);
		else if (kept.length > 0) out.push({ ...(g as Json), hooks: kept });
	}
	return out;
}

function eventGroups(hooks: Json, event: string): unknown[] {
	const current = hooks[event];
	if (current === undefined) return [];
	if (!Array.isArray(current)) {
		throw new Error(`\`hooks.${event}\` in Claude settings is not an array; nothing was written`);
	}
	return current;
}

function hooksObject(settings: Json): Json {
	if (settings.hooks === undefined) return {};
	if (!isObject(settings.hooks)) {
		throw new Error("`hooks` in Claude settings is not an object; nothing was written");
	}
	return settings.hooks;
}

/** The event holds exactly one luuna-atlas handler, in a group shaped as we write it. */
function isInstalled(groups: readonly unknown[], spec: HookSpec): boolean {
	const ours = groups.filter((g) => groupHooks(g).some(isAtlasHandler));
	return ours.length === 1 && JSON.stringify(ours[0]) === JSON.stringify(makeGroup(spec));
}

/** Events whose luuna-atlas handler is missing or out of date. */
export function missingAtlasHooks(settings: Json): string[] {
	const hooks = hooksObject(settings);
	return CLAUDE_HOOKS.filter((s) => !isInstalled(eventGroups(hooks, s.event), s)).map(
		(s) => s.event,
	);
}

/** Install (or with `remove`, strip) the luuna-atlas hooks. Does not mutate `settings`. */
export function syncAtlasHooks(
	settings: Json,
	remove: boolean,
): { settings: Json; changed: boolean } {
	const hooks: Json = structuredClone(hooksObject(settings));
	const events = remove ? Object.keys(hooks) : CLAUDE_HOOKS.map((s) => s.event);
	let changed = false;
	for (const event of events) {
		const groups = eventGroups(hooks, event);
		const spec = CLAUDE_HOOKS.find((s) => s.event === event);
		if (!remove && spec && isInstalled(groups, spec)) continue;
		const updated = stripAtlas(groups);
		if (!remove && spec) updated.push(makeGroup(spec));
		if (JSON.stringify(updated) === JSON.stringify(groups)) continue;
		changed = true;
		if (updated.length === 0) delete hooks[event];
		else hooks[event] = updated;
	}
	if (!changed) return { settings, changed: false };
	const next: Json = { ...settings, hooks };
	if (Object.keys(hooks).length === 0) delete next.hooks;
	return { settings: next, changed: true };
}
