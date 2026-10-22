import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, readFileSync } from "node:fs";
import { mkdir, mkdtemp, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { getAtlasDir, getConfigPath } from "../../src/utils/config.ts";
import {
	evidenceMissionRef,
	hasLegacyStore,
	migrateLegacyStore,
	resolveMissionsDir,
} from "../../src/utils/legacy-layout.ts";
import {
	hasMarkerSection,
	removeMarkerSection,
	replaceMarkerSection,
} from "../../src/utils/markers.ts";

const CLI_PATH = resolve(import.meta.dir, "..", "..", "src", "cli.ts");
const LAYOUT = { luunaDir: ".luuna", storeRel: ".luuna/atlas", configFile: "config.yaml" };

function runCli(args: string[], cwd: string) {
	return Bun.spawnSync(["bun", CLI_PATH, ...args], { cwd, stdout: "pipe", stderr: "pipe" });
}

async function makeLegacyStore(root: string): Promise<void> {
	await mkdir(join(root, ".mulch", "expertise"), { recursive: true });
	await writeFile(
		join(root, ".mulch", "mulch.config.yaml"),
		"version: '1'\ndomains: {}\n",
		"utf-8",
	);
	await writeFile(join(root, ".mulch", "expertise", "cli.jsonl"), "", "utf-8");
}

/** Create a directory symlink/junction, or return false when the host forbids it. */
async function trySymlink(target: string, link: string): Promise<boolean> {
	try {
		await symlink(target, link, "junction");
		return true;
	} catch {
		return false;
	}
}

describe("legacy layout migration", () => {
	let root: string;

	beforeEach(async () => {
		root = await mkdtemp(join(tmpdir(), "atlas-legacy-test-"));
	});

	afterEach(async () => {
		await rm(root, { recursive: true, force: true });
	});

	it("moves the legacy directory and renames the config file", async () => {
		await makeLegacyStore(root);
		await writeFile(
			join(root, ".gitattributes"),
			".mulch/expertise/*.jsonl merge=union\n",
			"utf-8",
		);

		expect(migrateLegacyStore(root, LAYOUT)).toBe(true);

		expect(existsSync(join(root, ".mulch"))).toBe(false);
		expect(existsSync(join(root, ".luuna", "atlas", "config.yaml"))).toBe(true);
		expect(existsSync(join(root, ".luuna", "atlas", "mulch.config.yaml"))).toBe(false);
		expect(existsSync(join(root, ".luuna", "atlas", "expertise", "cli.jsonl"))).toBe(true);
		expect(readFileSync(join(root, ".gitattributes"), "utf-8")).toBe(
			".luuna/atlas/expertise/*.jsonl merge=union\n",
		);
	});

	it("is a no-op when there is no legacy directory", () => {
		expect(migrateLegacyStore(root, LAYOUT)).toBe(false);
		expect(hasLegacyStore(root)).toBe(false);
	});

	it("fails with a clear error when both layouts exist", async () => {
		await makeLegacyStore(root);
		await mkdir(join(root, ".luuna", "atlas"), { recursive: true });

		expect(() => migrateLegacyStore(root, LAYOUT)).toThrow(/Both .* exist/);
		// Nothing was touched.
		expect(existsSync(join(root, ".mulch", "mulch.config.yaml"))).toBe(true);
	});

	it("refuses a symlinked legacy directory", async () => {
		const real = join(root, "real-store");
		await mkdir(real, { recursive: true });
		if (!(await trySymlink(real, join(root, ".mulch")))) return;

		expect(() => migrateLegacyStore(root, LAYOUT)).toThrow(/symbolic link/);
		expect(existsSync(join(root, ".luuna"))).toBe(false);
	});

	it("refuses a symlinked .luuna directory", async () => {
		await makeLegacyStore(root);
		const elsewhere = await mkdtemp(join(tmpdir(), "atlas-legacy-elsewhere-"));
		try {
			if (!(await trySymlink(elsewhere, join(root, ".luuna")))) return;
			expect(() => migrateLegacyStore(root, LAYOUT)).toThrow(/symbolic link/);
			expect(existsSync(join(root, ".mulch", "mulch.config.yaml"))).toBe(true);
		} finally {
			await rm(elsewhere, { recursive: true, force: true });
		}
	});

	it("getAtlasDir migrates transparently", async () => {
		await makeLegacyStore(root);
		const dir = getAtlasDir(root);
		expect(existsSync(dir)).toBe(true);
		expect(getConfigPath(root).endsWith("config.yaml")).toBe(true);
		expect(existsSync(getConfigPath(root))).toBe(true);
		expect(existsSync(join(root, ".mulch"))).toBe(false);
	});

	it("prints one notice on stderr and keeps stdout as clean JSON", async () => {
		await makeLegacyStore(root);
		const proc = runCli(["status", "--json"], root);
		const stdout = proc.stdout.toString();
		const stderr = proc.stderr.toString();

		expect(() => JSON.parse(stdout)).not.toThrow();
		expect(stdout).not.toContain("moved legacy");
		const notices = stderr.split("\n").filter((l) => l.includes("moved legacy"));
		expect(notices).toHaveLength(1);
		expect(existsSync(join(root, ".luuna", "atlas", "config.yaml"))).toBe(true);

		// A second run has nothing left to migrate.
		const again = runCli(["status", "--json"], root);
		expect(again.stderr.toString()).not.toContain("moved legacy");
	});

	it("errors from the CLI when both layouts exist", async () => {
		await makeLegacyStore(root);
		await mkdir(join(root, ".luuna", "atlas"), { recursive: true });
		const proc = runCli(["status"], root);
		expect(proc.exitCode).not.toBe(0);
		expect(proc.stderr.toString() + proc.stdout.toString()).toMatch(/Both .* exist/);
	});
});

describe("Missions data location", () => {
	let root: string;

	beforeEach(async () => {
		root = await mkdtemp(join(tmpdir(), "atlas-legacy-missions-"));
	});

	afterEach(async () => {
		await rm(root, { recursive: true, force: true });
	});

	it("prefers .luuna/missions", async () => {
		await mkdir(join(root, ".luuna", "missions"), { recursive: true });
		await mkdir(join(root, ".seeds"), { recursive: true });
		expect(resolveMissionsDir(root, ".luuna/missions")).toBe(".luuna/missions");
	});

	it("falls back to the legacy directory read-only", async () => {
		await mkdir(join(root, ".seeds"), { recursive: true });
		expect(resolveMissionsDir(root, ".luuna/missions")).toBe(".seeds");
	});

	it("defaults to the current location when neither exists", () => {
		expect(resolveMissionsDir(root, ".luuna/missions")).toBe(".luuna/missions");
	});
});

describe("legacy evidence and markers", () => {
	it("reads the Missions reference from the current or legacy evidence key", () => {
		expect(evidenceMissionRef({ missions: "m-1" })).toBe("m-1");
		expect(evidenceMissionRef({ seeds: "s-1" })).toBe("s-1");
		expect(evidenceMissionRef({ gh: "1" })).toBeUndefined();
		expect(evidenceMissionRef(undefined)).toBeUndefined();
	});

	it("recognises and replaces sections written with the legacy markers", () => {
		const content = "head\n<!-- mulch:start -->\nold\n<!-- mulch:end -->\ntail\n";
		expect(hasMarkerSection(content)).toBe(true);
		expect(replaceMarkerSection(content, "NEW")).toBe("head\nNEW\ntail\n");
		expect(removeMarkerSection(content)).toBe("head\n\ntail\n");
	});
});
