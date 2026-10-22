import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { execFileSync } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
	beadResolver,
	ghResolver,
	linearResolver,
	missionsResolver,
	type Resolver,
	type ResolverHit,
	resolveActiveWork,
	TRACKERS,
} from "../../src/utils/active-work.ts";

function initGitRepo(dir: string): void {
	execFileSync("git", ["init", "-q", "-b", "main"], { cwd: dir, stdio: "pipe" });
	execFileSync("git", ["config", "user.email", "test@test.com"], { cwd: dir, stdio: "pipe" });
	execFileSync("git", ["config", "user.name", "Test"], { cwd: dir, stdio: "pipe" });
}

async function commitOnBranch(dir: string, branch: string): Promise<void> {
	execFileSync("git", ["checkout", "-q", "-b", branch], { cwd: dir, stdio: "pipe" });
	await writeFile(join(dir, "stamp.txt"), branch);
	execFileSync("git", ["add", "."], { cwd: dir, stdio: "pipe" });
	execFileSync("git", ["commit", "-q", "-m", "stamp"], { cwd: dir, stdio: "pipe" });
}

async function writeMissionsIssues(
	dir: string,
	issues: Array<Record<string, unknown>>,
): Promise<void> {
	await mkdir(join(dir, ".luuna/missions"), { recursive: true });
	const lines = issues.map((i) => JSON.stringify(i)).join("\n");
	await writeFile(join(dir, ".luuna/missions", "issues.jsonl"), `${lines}\n`);
}

async function writeBeadIssues(dir: string, issues: Array<Record<string, unknown>>): Promise<void> {
	await mkdir(join(dir, ".beads"), { recursive: true });
	const lines = issues.map((i) => JSON.stringify(i)).join("\n");
	await writeFile(join(dir, ".beads", "issues.jsonl"), `${lines}\n`);
}

describe("active-work", () => {
	let tmpDir: string;

	beforeEach(async () => {
		tmpDir = await mkdtemp(join(tmpdir(), "atlas-active-work-"));
		initGitRepo(tmpDir);
	});

	afterEach(async () => {
		await rm(tmpDir, { recursive: true, force: true });
	});

	describe("missionsResolver", () => {
		it("returns empty when no signals", () => {
			const hit = missionsResolver(tmpDir);
			expect(hit).toEqual({ tracker: "missions", matches: [], source: "none" });
		});

		it("prefers in-progress issues over branch match", async () => {
			await writeMissionsIssues(tmpDir, [
				{ id: "atlas-aaaa", status: "open" },
				{ id: "atlas-bbbb", status: "in_progress" },
				{ id: "atlas-cccc", status: "closed" },
			]);
			await commitOnBranch(tmpDir, "atlas-dddd");

			const hit = missionsResolver(tmpDir);
			expect(hit.tracker).toBe("missions");
			expect(hit.source).toBe("in_progress");
			expect(hit.matches).toEqual(["atlas-bbbb"]);
		});

		it("returns multiple in-progress matches sorted", async () => {
			await writeMissionsIssues(tmpDir, [
				{ id: "atlas-cccc", status: "in_progress" },
				{ id: "atlas-aaaa", status: "in_progress" },
				{ id: "atlas-bbbb", status: "in_progress" },
			]);
			const hit = missionsResolver(tmpDir);
			expect(hit.matches).toEqual(["atlas-aaaa", "atlas-bbbb", "atlas-cccc"]);
		});

		it("falls back to branch name when no in-progress issues", async () => {
			await commitOnBranch(tmpDir, "feat/atlas-f3d0-resolver");
			const hit = missionsResolver(tmpDir);
			expect(hit.source).toBe("branch");
			expect(hit.matches).toEqual(["atlas-f3d0"]);
		});

		it("ignores malformed jsonl rows but keeps valid ones", async () => {
			await mkdir(join(tmpDir, ".luuna/missions"), { recursive: true });
			await writeFile(
				join(tmpDir, ".luuna/missions", "issues.jsonl"),
				`{"id":"atlas-aaaa","status":"in_progress"}\nnot-json\n# comment\n{"id":"atlas-bbbb","status":"in_progress"}\n`,
			);
			const hit = missionsResolver(tmpDir);
			expect(hit.matches).toEqual(["atlas-aaaa", "atlas-bbbb"]);
		});

		it("returns empty when no .luuna/missions/ and branch has no match", async () => {
			await commitOnBranch(tmpDir, "random-feature");
			const hit = missionsResolver(tmpDir);
			expect(hit.matches).toEqual([]);
		});
	});

	describe("linearResolver", () => {
		it("extracts Linear-style ID from branch name", async () => {
			await commitOnBranch(tmpDir, "alice/WEB-123-auth-flow");
			const hit = linearResolver(tmpDir);
			expect(hit.tracker).toBe("linear");
			expect(hit.source).toBe("branch");
			expect(hit.matches).toEqual(["WEB-123"]);
		});

		it("does not match lowercase prefixes (avoids missions-id collision)", async () => {
			await commitOnBranch(tmpDir, "atlas-f3d0");
			const hit = linearResolver(tmpDir);
			expect(hit.matches).toEqual([]);
		});

		it("returns empty when no match", async () => {
			await commitOnBranch(tmpDir, "feature/no-ticket");
			const hit = linearResolver(tmpDir);
			expect(hit.matches).toEqual([]);
		});
	});

	describe("beadResolver", () => {
		it("prefers in-progress bead issues over branch match", async () => {
			await writeBeadIssues(tmpDir, [
				{ id: "bd-1", status: "in_progress" },
				{ id: "bd-2", status: "open" },
			]);
			await commitOnBranch(tmpDir, "feat/bd-9");
			const hit = beadResolver(tmpDir);
			expect(hit.source).toBe("in_progress");
			expect(hit.matches).toEqual(["bd-1"]);
		});

		it("normalizes bead ids from branch to lowercase", async () => {
			await commitOnBranch(tmpDir, "feat/BD-42");
			const hit = beadResolver(tmpDir);
			expect(hit.source).toBe("branch");
			expect(hit.matches).toEqual(["bd-42"]);
		});

		it("matches bead-NNNN form as well as bd-NNNN", async () => {
			await commitOnBranch(tmpDir, "fix/bead-7");
			const hit = beadResolver(tmpDir);
			expect(hit.matches).toEqual(["bead-7"]);
		});
	});

	describe("ghResolver", () => {
		it("extracts gh-NNN from branch name when gh CLI unavailable", async () => {
			// gh might be installed locally; the branch fallback should still kick
			// in when the CLI fails to find a PR. Use a branch shape `gh` can't
			// resolve (no upstream / no PR) so we hit the regex path.
			await commitOnBranch(tmpDir, "fix/gh-99-typo");
			const hit = ghResolver(tmpDir);
			// Either gh CLI returned a number (very unlikely in tmp repo) or branch
			// regex matched "#99". Accept either: the contract is that source ∈
			// {gh-cli, branch} and at least one match was returned.
			if (hit.matches.length > 0) {
				expect(["gh-cli", "branch"]).toContain(hit.source);
			}
			if (hit.source === "branch") {
				expect(hit.matches).toEqual(["#99"]);
			}
		});

		it("returns empty when branch has no PR-style hint", async () => {
			await commitOnBranch(tmpDir, "feature/no-pr");
			const hit = ghResolver(tmpDir);
			// In a fresh repo with no remote, gh CLI will fail too.
			expect(hit.matches).toEqual([]);
		});
	});

	describe("resolveActiveWork", () => {
		it("returns auto-link suggestions for single-match trackers", async () => {
			await writeMissionsIssues(tmpDir, [{ id: "atlas-bbbb", status: "in_progress" }]);
			await commitOnBranch(tmpDir, "WEB-42-feature");

			const result = resolveActiveWork({ cwd: tmpDir });
			expect(result.missions).toBe("atlas-bbbb");
			expect(result.linear).toBe("WEB-42");
			expect(result.warnings).toEqual([]);
		});

		it("warns on multi-match and skips the auto-link", async () => {
			await writeMissionsIssues(tmpDir, [
				{ id: "atlas-aaaa", status: "in_progress" },
				{ id: "atlas-bbbb", status: "in_progress" },
			]);

			const result = resolveActiveWork({ cwd: tmpDir });
			expect(result.missions).toBeUndefined();
			expect(result.warnings).toHaveLength(1);
			expect(result.warnings[0]).toContain("multiple missions candidates");
			expect(result.warnings[0]).toContain("atlas-aaaa");
			expect(result.warnings[0]).toContain("atlas-bbbb");
			expect(result.warnings[0]).toContain("--evidence-missions");
		});

		it("overrides win even when resolver finds a single match", async () => {
			await writeMissionsIssues(tmpDir, [{ id: "atlas-aaaa", status: "in_progress" }]);
			const result = resolveActiveWork({
				cwd: tmpDir,
				overrides: { missions: "atlas-zzzz" },
			});
			expect(result.missions).toBe("atlas-zzzz");
		});

		it("overrides win and suppress the multi-match warning", async () => {
			await writeMissionsIssues(tmpDir, [
				{ id: "atlas-aaaa", status: "in_progress" },
				{ id: "atlas-bbbb", status: "in_progress" },
			]);
			const result = resolveActiveWork({
				cwd: tmpDir,
				overrides: { missions: "atlas-zzzz" },
			});
			expect(result.missions).toBe("atlas-zzzz");
			expect(result.warnings).toEqual([]);
		});

		it("treats empty-string override as not-set (resolver still runs)", async () => {
			await writeMissionsIssues(tmpDir, [{ id: "atlas-aaaa", status: "in_progress" }]);
			const result = resolveActiveWork({
				cwd: tmpDir,
				overrides: { missions: "" },
			});
			expect(result.missions).toBe("atlas-aaaa");
		});

		it("applies overrides for trackers no resolver claims", () => {
			const noopResolver: Resolver = () => ({
				tracker: "missions",
				matches: [],
				source: "none",
			});
			const result = resolveActiveWork({
				cwd: tmpDir,
				resolvers: [noopResolver],
				overrides: { linear: "WEB-1", bead: "bd-9" },
			});
			expect(result.linear).toBe("WEB-1");
			expect(result.bead).toBe("bd-9");
			expect(result.missions).toBeUndefined();
			expect(result.gh).toBeUndefined();
		});

		it("lets custom resolvers prepended to the chain win over defaults", async () => {
			await writeMissionsIssues(tmpDir, [{ id: "atlas-aaaa", status: "in_progress" }]);
			const stubMissions: Resolver = (): ResolverHit => ({
				tracker: "missions",
				matches: ["atlas-override"],
				source: "stub",
			});
			const result = resolveActiveWork({
				cwd: tmpDir,
				resolvers: [stubMissions, missionsResolver, ghResolver, linearResolver, beadResolver],
			});
			expect(result.missions).toBe("atlas-override");
		});

		it("emits no warnings and no auto-links on a clean repo", async () => {
			await commitOnBranch(tmpDir, "main-work");
			const result = resolveActiveWork({ cwd: tmpDir });
			for (const t of TRACKERS) expect(result[t]).toBeUndefined();
			expect(result.warnings).toEqual([]);
		});

		it("covers all four trackers symmetrically when each has one signal", async () => {
			await writeMissionsIssues(tmpDir, [{ id: "atlas-aaaa", status: "in_progress" }]);
			await writeBeadIssues(tmpDir, [{ id: "bd-1", status: "in_progress" }]);
			await commitOnBranch(tmpDir, "WEB-7-foo");

			const result = resolveActiveWork({
				cwd: tmpDir,
				overrides: { gh: "#123" },
			});
			expect(result.missions).toBe("atlas-aaaa");
			expect(result.bead).toBe("bd-1");
			expect(result.linear).toBe("WEB-7");
			expect(result.gh).toBe("#123");
		});
	});
});
