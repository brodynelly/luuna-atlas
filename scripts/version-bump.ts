#!/usr/bin/env bun
/** Keep current native/compatibility versions synchronized; never change the frozen oracle. */
import { randomUUID } from "node:crypto";
import { readFileSync, renameSync, statSync, unlinkSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

export type BumpLevel = "major" | "minor" | "patch";
interface VersionChange {
	path: string;
	before: string;
	after: string;
	mode: number;
}
export interface VersionBump {
	current: string;
	next: string;
	changes: VersionChange[];
}
interface StagedChange extends VersionChange {
	temp: string;
	backup: string;
}

function restoreVersionFiles(files: StagedChange[]): unknown[] {
	const failures: unknown[] = [];
	for (const change of [...files].reverse()) {
		try {
			renameSync(change.backup, change.path);
		} catch (failure) {
			failures.push(failure);
		}
	}
	return failures;
}

function cleanVersionFiles(files: StagedChange[], complete: boolean): void {
	for (const change of files) {
		try {
			unlinkSync(change.temp);
		} catch {
			/* Renamed or never staged. */
		}
		try {
			// Preserve a recovery backup if rollback failed.
			if (complete || readFileSync(change.path, "utf8") === change.before)
				unlinkSync(change.backup);
		} catch {
			/* Preserve recovery data on failure. */
		}
	}
}

function replaceConstant(
	text: string,
	pattern: RegExp,
	current: string,
	next: string,
	path: string,
): string {
	const matches = [...text.matchAll(pattern)];
	if (matches.length !== 1 || matches[0]?.[2] !== current) {
		throw new Error(`Expected one version constant matching ${current} in ${path}`);
	}
	return text.replace(
		pattern,
		(_match, prefix: string, _value: string, suffix: string) => `${prefix}${next}${suffix}`,
	);
}

/** Read and validate every target before preparing any writes. */
export function planVersionBump(root: string, level: BumpLevel): VersionBump {
	if (!["major", "minor", "patch"].includes(level)) throw new Error("Invalid version bump level");
	const read = (path: string): VersionChange => {
		const absolute = resolve(root, path);
		const before = readFileSync(absolute, "utf8");
		return { path: absolute, before, after: before, mode: statSync(absolute).mode };
	};
	const pkg = read("package.json");
	const current: unknown = JSON.parse(pkg.before).version;
	if (typeof current !== "string" || !/^\d+\.\d+\.\d+$/.test(current)) {
		throw new Error(`Invalid version in package.json: ${String(current)}`);
	}
	const parts = current.split(".").map(Number) as [number, number, number];
	if (level === "major") {
		parts[0]++;
		parts[1] = 0;
		parts[2] = 0;
	} else if (level === "minor") {
		parts[1]++;
		parts[2] = 0;
	} else parts[2]++;
	if (parts.some((part) => !Number.isSafeInteger(part)))
		throw new Error("Version exceeds supported integer range");
	const next = parts.join(".");
	pkg.after = replaceConstant(
		pkg.before,
		/("version"\s*:\s*")([^"\n]+)(")/g,
		current,
		next,
		pkg.path,
	);
	const cli = read("src/cli.ts");
	cli.after = replaceConstant(
		cli.before,
		/(\bconst VERSION\s*=\s*")([^"\n]+)(")/g,
		current,
		next,
		cli.path,
	);
	const native = read("internal/atlas/app.go");
	native.after = replaceConstant(
		native.before,
		/(\bconst Version\s*=\s*")([^"\n]+)(")/g,
		current,
		next,
		native.path,
	);
	const contractFile = read("internal/atlas/contract.json");
	const contract = JSON.parse(contractFile.before) as { version: string; cli: unknown };
	if (contract.version !== current)
		throw new Error(`Expected contract version ${current}, found ${contract.version}`);
	function updateHelp(value: unknown): void {
		if (!value || typeof value !== "object")
			throw new Error("Invalid command in compatibility contract");
		const command = value as { help?: string; commands?: unknown[] };
		const marker = ` v${current} — Structured expertise management`;
		if (typeof command.help !== "string" || !command.help.split("\n", 1)[0]?.endsWith(marker)) {
			throw new Error(`Expected help version ${current} in compatibility contract`);
		}
		command.help = command.help.replace(marker, ` v${next} — Structured expertise management`);
		if (!Array.isArray(command.commands))
			throw new Error("Missing commands in compatibility contract");
		for (const child of command.commands) updateHelp(child);
	}
	updateHelp(contract.cli);
	contract.version = next;
	contractFile.after = `${JSON.stringify(contract, null, 2)}\n`;
	return { current, next, changes: [pkg, cli, native, contractFile] };
}

/** Stage all files first; restore every replaced original if a later rename fails. */
export function writeVersionBump(
	plan: VersionBump,
	replaceFile: typeof renameSync = renameSync,
): void {
	const suffix = `.version-bump-${process.pid}-${randomUUID()}`;
	const staged: StagedChange[] = plan.changes.map((change) => ({
		...change,
		temp: `${change.path}${suffix}.next`,
		backup: `${change.path}${suffix}.before`,
	}));
	const replaced: typeof staged = [];
	let complete = false;
	try {
		for (const change of staged) {
			if (readFileSync(change.path, "utf8") !== change.before)
				throw new Error(`File changed during version bump: ${change.path}`);
		}
		for (const change of staged) {
			writeFileSync(change.temp, change.after, { flag: "wx", mode: change.mode });
			writeFileSync(change.backup, change.before, { flag: "wx", mode: change.mode });
		}
		for (const change of staged) {
			replaceFile(change.temp, change.path);
			replaced.push(change);
		}
		complete = true;
	} catch (error) {
		const failures = restoreVersionFiles(replaced);
		if (failures.length)
			throw new AggregateError(
				[error, ...failures],
				"Version bump failed; some originals could not be restored. Preserve .before files for recovery.",
			);
		throw error;
	} finally {
		cleanVersionFiles(staged, complete);
	}
}

if (import.meta.main) {
	const level = Bun.argv[2];
	if (level !== "major" && level !== "minor" && level !== "patch") {
		console.error("Usage: bun run scripts/version-bump.ts <major|minor|patch>");
		process.exit(1);
	}
	try {
		const plan = planVersionBump(resolve(import.meta.dir, ".."), level);
		writeVersionBump(plan);
		console.log(`Bumped ${plan.current} → ${plan.next}`);
		console.log("");
		console.log("Next steps:");
		console.log(`  1. Update CHANGELOG.md — move [Unreleased] items to [${plan.next}]`);
		console.log(
			"  2. Review package.json, src/cli.ts, internal/atlas/app.go and internal/atlas/contract.json",
		);
		console.log("  3. Run bun run verify; leave compat/reference unchanged");
		console.log("  4. Follow the separate reviewed release process in UPSTREAM.md");
	} catch (error) {
		console.error(error instanceof Error ? error.message : String(error));
		process.exit(1);
	}
}
