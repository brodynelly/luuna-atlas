import { afterEach, describe, expect, it } from "bun:test";
import {
	mkdirSync,
	mkdtempSync,
	readdirSync,
	readFileSync,
	renameSync,
	rmSync,
	writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { planVersionBump, writeVersionBump } from "../../scripts/version-bump.ts";

const temporary: string[] = [];
afterEach(() => {
	for (const root of temporary.splice(0)) rmSync(root, { recursive: true, force: true });
});
function fixture(): string {
	const root = mkdtempSync(join(tmpdir(), "atlas-version-"));
	temporary.push(root);
	const command = (name: string, commands: unknown[] = []) => ({
		name,
		help: `${name} v0.11.0 — Structured expertise management\n`,
		commands,
	});
	const files: Record<string, string> = {
		"package.json": '{\n  "version": "0.11.0",\n  "private": true\n}\n',
		"src/cli.ts": 'export const VERSION = "0.11.0";\n',
		"internal/atlas/app.go": 'package atlas\nconst Version = "0.11.0"\n',
		"internal/atlas/contract.json": `${JSON.stringify({ version: "0.11.0", baseline: "frozen", cli: command("Luuna Atlas", [command("config", [command("set")])]) }, null, 2)}\n`,
		"compat/reference/package.json": '{"version":"0.11.0"}',
	};
	for (const [path, content] of Object.entries(files)) {
		mkdirSync(dirname(join(root, path)), { recursive: true });
		writeFileSync(join(root, path), content);
	}
	return root;
}

describe("native version maintenance", () => {
	it("updates all current versions and nested help while preserving the frozen oracle", () => {
		const root = fixture();
		const plan = planVersionBump(root, "minor");
		expect(plan.next).toBe("0.12.0");
		writeVersionBump(plan);
		for (const file of plan.changes) expect(readFileSync(file.path, "utf8")).toBe(file.after);
		expect(readFileSync(join(root, "compat/reference/package.json"), "utf8")).toBe(
			'{"version":"0.11.0"}',
		);
		const contract = JSON.parse(readFileSync(join(root, "internal/atlas/contract.json"), "utf8"));
		expect(contract.cli.commands[0].commands[0].help).toContain("v0.12.0");
		expect(contract.baseline).toBe("frozen");
		expect(readdirSync(join(root, "internal/atlas")).sort()).toEqual(["app.go", "contract.json"]);
	});
	it("validates the complete plan before writing", () => {
		const root = fixture();
		writeFileSync(join(root, "internal/atlas/app.go"), 'package atlas\nconst Version = "9.9.9"\n');
		expect(() => planVersionBump(root, "patch")).toThrow("Expected one version constant");
		expect(JSON.parse(readFileSync(join(root, "package.json"), "utf8")).version).toBe("0.11.0");
		expect(readFileSync(join(root, "src/cli.ts"), "utf8")).toContain("0.11.0");
	});
	it("restores earlier files if a later replacement fails", () => {
		const root = fixture();
		const plan = planVersionBump(root, "patch");
		let replacements = 0;
		expect(() =>
			writeVersionBump(plan, (source, destination) => {
				if (++replacements === 3) throw new Error("simulated rename failure");
				renameSync(source, destination);
			}),
		).toThrow("simulated rename failure");
		for (const file of plan.changes) expect(readFileSync(file.path, "utf8")).toBe(file.before);
		expect(readdirSync(join(root, "internal/atlas")).sort()).toEqual(["app.go", "contract.json"]);
	});
	it("rejects edits made after planning without overwriting them", () => {
		const root = fixture();
		const plan = planVersionBump(root, "major");
		writeFileSync(join(root, "src/cli.ts"), "changed by another editor\n");
		expect(() => writeVersionBump(plan)).toThrow("File changed during version bump");
		expect(readFileSync(join(root, "src/cli.ts"), "utf8")).toBe("changed by another editor\n");
		expect(JSON.parse(readFileSync(join(root, "package.json"), "utf8")).version).toBe("0.11.0");
	});
});
