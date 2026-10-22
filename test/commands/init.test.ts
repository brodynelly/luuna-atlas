import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync } from "node:fs";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
	GITATTRIBUTES_LINE,
	getAtlasDir,
	getConfigPath,
	getExpertiseDir,
	initAtlasDir,
	readConfig,
	writeConfig,
} from "../../src/utils/config.ts";

describe("init command", () => {
	let tmpDir: string;

	beforeEach(async () => {
		tmpDir = await mkdtemp(join(tmpdir(), "atlas-init-test-"));
	});

	afterEach(async () => {
		await rm(tmpDir, { recursive: true, force: true });
	});

	it("creates .luuna/atlas/ with config and expertise/", async () => {
		await initAtlasDir(tmpDir);

		expect(existsSync(getAtlasDir(tmpDir))).toBe(true);
		expect(existsSync(getConfigPath(tmpDir))).toBe(true);
		expect(existsSync(getExpertiseDir(tmpDir))).toBe(true);
	});

	it("creates a valid default config", async () => {
		await initAtlasDir(tmpDir);

		const config = await readConfig(tmpDir);
		expect(config.version).toBe("1");
		expect(config.domains).toEqual({});
		expect(config.governance.max_entries).toBe(100);
		expect(config.governance.warn_entries).toBe(150);
		expect(config.governance.hard_limit).toBe(200);
	});

	it("running init twice does not error", async () => {
		await initAtlasDir(tmpDir);

		// Second init should succeed without throwing
		await expect(initAtlasDir(tmpDir)).resolves.toBeUndefined();

		// Config should still be valid after second init
		const config = await readConfig(tmpDir);
		expect(config.version).toBe("1");
	});

	it("re-running init preserves customized config", async () => {
		await initAtlasDir(tmpDir);

		// Customize the config
		const config = await readConfig(tmpDir);
		config.domains = { "custom-domain": {} };
		config.governance.max_entries = 50;
		await writeConfig(config, tmpDir);

		// Re-run init
		await initAtlasDir(tmpDir);

		// Config should retain customizations
		const after = await readConfig(tmpDir);
		expect(after.domains).toEqual({ "custom-domain": {} });
		expect(after.governance.max_entries).toBe(50);
	});

	it("checks that .luuna/atlas/ already exists", () => {
		// Before init, directory should not exist
		expect(existsSync(getAtlasDir(tmpDir))).toBe(false);
	});

	it("creates .gitattributes with merge=union for JSONL files", async () => {
		await initAtlasDir(tmpDir);

		const content = await readFile(join(tmpDir, ".gitattributes"), "utf-8");
		expect(content).toContain(GITATTRIBUTES_LINE);
	});

	it("appends to existing .gitattributes without overwriting", async () => {
		const existing = "*.png binary\n";
		await writeFile(join(tmpDir, ".gitattributes"), existing, "utf-8");

		await initAtlasDir(tmpDir);

		const content = await readFile(join(tmpDir, ".gitattributes"), "utf-8");
		expect(content).toContain("*.png binary");
		expect(content).toContain(GITATTRIBUTES_LINE);
	});

	it("does not duplicate gitattributes line on second init", async () => {
		await initAtlasDir(tmpDir);
		await initAtlasDir(tmpDir);

		const content = await readFile(join(tmpDir, ".gitattributes"), "utf-8");
		const occurrences = content.split(GITATTRIBUTES_LINE).length - 1;
		expect(occurrences).toBe(1);
	});

	it("creates .luuna/atlas/README.md", async () => {
		await initAtlasDir(tmpDir);

		const readmePath = join(getAtlasDir(tmpDir), "README.md");
		expect(existsSync(readmePath)).toBe(true);
	});

	it("README.md contains repo URL and key commands", async () => {
		await initAtlasDir(tmpDir);

		const content = await readFile(join(getAtlasDir(tmpDir), "README.md"), "utf-8");
		expect(content).toContain("https://github.com/brodynelly/luuna-atlas");
		expect(content).toContain("luuna-atlas record");
		expect(content).toContain("luuna-atlas query");
		expect(content).toContain("luuna-atlas prime");
	});

	it("scaffolds config with commented-out optional knobs", async () => {
		await initAtlasDir(tmpDir);

		const content = await readFile(getConfigPath(tmpDir), "utf-8");
		// Header comment surfaces the docs link and the comment-stripping caveat
		expect(content).toContain("# Atlas configuration");
		expect(content).toContain("https://github.com/brodynelly/luuna-atlas");
		// Optional knob is present as commented YAML so users can discover it
		expect(content).toContain("# prime:");
		expect(content).toContain("#   default_mode: full");
		// Required fields are real YAML, not commented out
		expect(content).toMatch(/^version: '1'$/m);
		expect(content).toMatch(/^governance:$/m);
	});

	it("scaffolded config still parses to default values", async () => {
		await initAtlasDir(tmpDir);

		const config = await readConfig(tmpDir);
		expect(config.version).toBe("1");
		expect(config.domains).toEqual({});
		expect(config.governance.max_entries).toBe(100);
		expect(config.classification_defaults.shelf_life.tactical).toBe(14);
		expect(config.prime).toBeUndefined();
	});
});
