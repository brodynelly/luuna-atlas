// `luuna-atlas hook` (atlas-7164): PreToolUse guard + file-anchored injection.
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { hookOutput } from "../../src/commands/hook.ts";
import { DEFAULT_CONFIG } from "../../src/schemas/config.ts";
import type { ExpertiseRecord } from "../../src/schemas/record.ts";
import { getExpertisePath, initAtlasDir, writeConfig } from "../../src/utils/config.ts";
import { appendRecord } from "../../src/utils/expertise.ts";
import { bashWritesExpertise, guardDecision } from "../../src/utils/hook-guard.ts";
import { findRepoRoot } from "../../src/utils/hook-inject.ts";

const cliPath = resolve(process.cwd(), "src/cli.ts");

describe("hook guard", () => {
	const cwd = "/repo";
	const edit = (tool: string, key: string, p: string) =>
		guardDecision({ tool_name: tool, tool_input: { [key]: p } }, cwd);

	it("denies file tools on .luuna/atlas/expertise/*.jsonl only", () => {
		expect(edit("Edit", "file_path", ".luuna/atlas/expertise/cli.jsonl")).toContain(
			"luuna-atlas record",
		);
		expect(edit("Write", "file_path", "/repo/.luuna/atlas/expertise/db.jsonl")).toContain(
			"luuna-atlas edit",
		);
		expect(edit("NotebookEdit", "notebook_path", ".luuna/atlas/expertise/x.jsonl")).not.toBeNull();
		expect(edit("Edit", "file_path", ".luuna/atlas/config.yaml")).toBeNull();
		expect(edit("Edit", "file_path", "src/expertise/cli.jsonl")).toBeNull();
		expect(edit("Read", "file_path", ".luuna/atlas/expertise/cli.jsonl")).toBeNull();
		expect(edit("Edit", "file_path", "")).toBeNull();
	});

	it("catches shell writes, allows reads and luuna-atlas itself", () => {
		const denied = [
			"echo '{}' >> .luuna/atlas/expertise/cli.jsonl",
			"jq -c . a.json > .luuna/atlas/expertise/cli.jsonl",
			"sed -i '' 's/a/b/' .luuna/atlas/expertise/cli.jsonl",
			"cat x | tee -a .luuna/atlas/expertise/cli.jsonl",
			"cp /tmp/x.jsonl .luuna/atlas/expertise/",
			"FOO=1 perl -pi -e 's/a/b/' .luuna/atlas/expertise/cli.jsonl",
			"dd if=/dev/null of=.luuna/atlas/expertise/cli.jsonl",
			"rm .luuna/atlas/expertise/cli.jsonl",
		];
		for (const cmd of denied) expect(bashWritesExpertise(cmd)).toBe(true);
		const allowed = [
			"cat .luuna/atlas/expertise/cli.jsonl",
			"grep -c mx- .luuna/atlas/expertise/*.jsonl",
			"jq -c '.id' .luuna/atlas/expertise/cli.jsonl 2>&1 | head",
			"cp .luuna/atlas/expertise/cli.jsonl /tmp/backup.jsonl",
			"luuna-atlas record cli --type convention --description 'never > .luuna/atlas/expertise/x.jsonl'",
			"git checkout .luuna/atlas/expertise/cli.jsonl",
			"echo hi > out.txt",
		];
		for (const cmd of allowed) expect(bashWritesExpertise(cmd)).toBe(false);
		expect(guardDecision({ tool_name: "Bash", tool_input: { command: denied[0] } }, cwd)).toContain(
			"do not edit",
		);
		expect(guardDecision({ tool_name: "Bash", tool_input: {} }, cwd)).toBeNull();
	});
});

describe("luuna-atlas hook", () => {
	let tmpDir: string;
	const now = new Date().toISOString();

	beforeEach(async () => {
		tmpDir = await mkdtemp(join(tmpdir(), "atlas-hook-test-"));
		await initAtlasDir(tmpDir);
		await writeConfig({ ...DEFAULT_CONFIG, domains: { cli: {}, db: {} } }, tmpDir);
		await mkdir(join(tmpDir, "src"), { recursive: true });
		await writeFile(join(tmpDir, "src", "a.ts"), "x");
		const rec = (r: Record<string, unknown>) =>
			({ classification: "foundational", recorded_at: now, ...r }) as unknown as ExpertiseRecord;
		const cli = getExpertisePath("cli", tmpDir);
		await appendRecord(
			cli,
			rec({ type: "pattern", name: "p", description: "Anchored", id: "mx-1", files: ["src/a.ts"] }),
		);
		await appendRecord(cli, rec({ type: "convention", content: "Universal rule", id: "mx-2" }));
		await appendRecord(
			getExpertisePath("db", tmpDir),
			rec({
				type: "failure",
				description: "Dir failure",
				resolution: "Fix it",
				id: "mx-3",
				dir_anchors: ["src"],
			}),
		);
		await appendRecord(
			cli,
			rec({ type: "pattern", name: "old", description: "Old", id: "mx-4", files: ["src/a.ts"] }),
		);
		await appendRecord(
			cli,
			rec({ type: "convention", content: "Replaces old", id: "mx-5", supersedes: ["mx-4"] }),
		);
	});

	afterEach(async () => {
		await rm(tmpDir, { recursive: true, force: true });
	});

	const input = (tool: string, file: string, extra: Record<string, unknown> = {}) =>
		JSON.stringify({
			session_id: "sess/1",
			cwd: tmpDir,
			hook_event_name: "PreToolUse",
			tool_name: tool,
			tool_input: { file_path: file },
			...extra,
		});

	it("injects anchored records once per session and logs usage", async () => {
		const out = await hookOutput(input("Read", join(tmpDir, "src", "a.ts")), "/");
		const ctx = JSON.parse(out).hookSpecificOutput;
		expect(ctx.hookEventName).toBe("PreToolUse");
		expect(ctx.permissionDecision).toBeUndefined();
		const text: string = ctx.additionalContext;
		expect(text).toContain("records anchored to src/a.ts");
		expect(text.indexOf("mx-3")).toBeLessThan(text.indexOf("mx-1")); // failures first
		expect(text).not.toContain("mx-2"); // unanchored: SessionStart's job
		expect(text).not.toContain("mx-4"); // superseded

		expect(await hookOutput(input("Edit", "src/a.ts"), "/")).toBe("");
		const other = await hookOutput(input("Read", "src/a.ts", { session_id: "s2" }), "/");
		expect(other).toContain("mx-1");

		const state = join(tmpDir, ".luuna/atlas", "state");
		expect(await readFile(join(state, ".gitignore"), "utf-8")).toBe("*\n");
		const usage = (await readFile(join(state, "usage.jsonl"), "utf-8"))
			.trim()
			.split("\n")
			.map((l) => JSON.parse(l));
		expect(usage).toHaveLength(2);
		expect(usage[0]).toMatchObject({ session: "sess/1", tool: "Read", files: ["src/a.ts"] });
		expect(usage[0].ids.sort()).toEqual(["mx-1", "mx-3"]);
		expect(existsSync(join(state, "sessions", "sess_1.txt"))).toBe(true);
	});

	it("is silent when nothing is anchored, outside the repo, or for other events", async () => {
		await writeFile(join(tmpDir, "README.md"), "x");
		expect(await hookOutput(input("Read", "README.md"), "/")).toBe("");
		expect(await hookOutput(input("Read", "/elsewhere/a.ts"), "/")).toBe("");
		expect(await hookOutput(input("Read", ".luuna/atlas/expertise/cli.jsonl"), "/")).toBe("");
		const start = input("Read", "src/a.ts", { hook_event_name: "SessionStart" });
		expect(await hookOutput(start, "/")).toBe("");
		expect(await hookOutput(input("Glob", "src/a.ts"), "/")).toBe("");
		expect(await hookOutput("not json", "/")).toBe("");
		expect(existsSync(join(tmpDir, ".luuna/atlas", "state"))).toBe(false);
	});

	it("is silent without a .luuna/atlas project", async () => {
		const bare = await mkdtemp(join(tmpdir(), "atlas-hook-bare-"));
		try {
			const raw = JSON.stringify({ cwd: bare, tool_name: "Read", tool_input: { file_path: "a" } });
			expect(await hookOutput(raw, "/")).toBe("");
		} finally {
			await rm(bare, { recursive: true, force: true });
		}
	});

	it("denies hand edits end to end through the CLI", () => {
		const raw = input("Write", join(tmpDir, ".luuna/atlas", "expertise", "cli.jsonl"));
		const r = spawnSync("bun", [cliPath, "hook"], {
			cwd: tmpDir,
			input: raw,
			encoding: "utf-8",
			timeout: 8000,
		});
		expect(r.status).toBe(0);
		const out = JSON.parse(r.stdout).hookSpecificOutput;
		expect(out.permissionDecision).toBe("deny");
		expect(out.permissionDecisionReason).toContain("luuna-atlas record");
	});

	it("findRepoRoot walks up to the .git entry", async () => {
		await writeFile(join(tmpDir, ".git"), "gitdir: elsewhere");
		expect(findRepoRoot(join(tmpDir, "src"))).toBe(resolve(tmpDir));
	});
});
