// `luuna-atlas setup claude` settings merge (atlas-7164).
import { describe, expect, it } from "bun:test";
import { CLAUDE_HOOKS, missingAtlasHooks, syncAtlasHooks } from "../../src/utils/claude-hooks.ts";

const other = { matcher: "Bash", hooks: [{ type: "command", command: "other-guard" }] };

describe("syncAtlasHooks", () => {
	it("installs SessionStart prime + PreToolUse hook and keeps unrelated hooks", () => {
		const { settings, changed } = syncAtlasHooks(
			{ model: "x", hooks: { PreToolUse: [other] } },
			false,
		);
		expect(changed).toBe(true);
		expect(settings.model).toBe("x");
		const hooks = settings.hooks as Record<string, unknown[]>;
		expect(hooks.PreToolUse).toEqual([
			other,
			{
				matcher: "Read|Edit|Write|MultiEdit|NotebookEdit|Bash",
				hooks: [{ type: "command", command: "luuna-atlas hook" }],
			},
		]);
		expect(hooks.SessionStart).toEqual([
			{ matcher: "", hooks: [{ type: "command", command: "luuna-atlas prime" }] },
		]);
		expect(missingAtlasHooks(settings)).toEqual([]);
		expect(syncAtlasHooks(settings, false).changed).toBe(false);
	});

	it("replaces stale luuna-atlas handlers instead of duplicating them", () => {
		const old = {
			hooks: {
				SessionStart: [
					{
						matcher: "",
						hooks: [
							{ type: "command", command: "luuna-atlas prime --full" },
							{ type: "command", command: "keep-me" },
						],
					},
				],
			},
		};
		expect(missingAtlasHooks(old)).toEqual(["SessionStart", "PreToolUse"]);
		const { settings } = syncAtlasHooks(old, false);
		const start = (settings.hooks as Record<string, unknown[]>).SessionStart;
		expect(start).toEqual([
			{ matcher: "", hooks: [{ type: "command", command: "keep-me" }] },
			{ matcher: "", hooks: [{ type: "command", command: "luuna-atlas prime" }] },
		]);
	});

	it("remove strips luuna-atlas handlers everywhere and drops empty events", () => {
		const installed = syncAtlasHooks({ hooks: { PreToolUse: [other] } }, false).settings;
		const withLegacy = {
			...installed,
			hooks: {
				...(installed.hooks as object),
				PreCompact: [{ matcher: "", hooks: [{ type: "command", command: "luuna-atlas prime" }] }],
			},
		};
		const { settings, changed } = syncAtlasHooks(withLegacy, true);
		expect(changed).toBe(true);
		expect(settings.hooks).toEqual({ PreToolUse: [other] });
		expect(syncAtlasHooks({}, true)).toEqual({ settings: {}, changed: false });
		expect(syncAtlasHooks(syncAtlasHooks({}, false).settings, true).settings).toEqual({});
	});

	it("refuses malformed hook shapes", () => {
		expect(() => syncAtlasHooks({ hooks: [] }, false)).toThrow("not an object");
		expect(() => syncAtlasHooks({ hooks: { PreToolUse: {} } }, false)).toThrow("not an array");
		expect(() => missingAtlasHooks({ hooks: "x" })).toThrow("not an object");
	});

	it("ships exactly two handlers", () => {
		expect(CLAUDE_HOOKS.map((h) => h.command)).toEqual(["luuna-atlas prime", "luuna-atlas hook"]);
	});
});
