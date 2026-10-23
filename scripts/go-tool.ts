#!/usr/bin/env bun
/** Development toolchain discovery. The installed native CLI does not use this. */
import { existsSync, readdirSync } from "node:fs";
import { delimiter, dirname, join, resolve } from "node:path";

export const GO_VERSION = "1.27.0";
export const repositoryRoot = resolve(import.meta.dir, "..");

export function goEnvironment(): NodeJS.ProcessEnv {
	return {
		...process.env,
		PATH: [dirname(process.execPath), process.env.PATH ?? ""].join(delimiter),
		GOTOOLCHAIN: "local",
		GOPROXY: "off",
		GOSUMDB: "off",
	};
}

export function resolveGo(): string {
	const candidates = [
		process.env.LUUNA_ATLAS_GO,
		Bun.which("go"),
		process.platform === "win32" ? "C:\\Program Files\\Go\\bin\\go.exe" : undefined,
	];
	const executable = candidates.find((candidate) => candidate && existsSync(candidate));
	if (!executable) {
		throw new Error(
			`Development requires Go ${GO_VERSION}. Install it or set LUUNA_ATLAS_GO to its executable.`,
		);
	}
	const version = Bun.spawnSync([executable, "version"], {
		cwd: repositoryRoot,
		env: goEnvironment(),
		stdout: "pipe",
		stderr: "pipe",
	});
	const actual = /\bgo(\d+\.\d+\.\d+)\b/.exec(version.stdout.toString())?.[1];
	if (version.exitCode !== 0 || actual !== GO_VERSION) {
		throw new Error(
			`Development requires Go ${GO_VERSION}; ${executable} reports ${actual ?? "an unavailable version"}.`,
		);
	}
	return executable;
}

export function runGo(args: readonly string[], executable = resolveGo()): void {
	const result = Bun.spawnSync([executable, ...args], {
		cwd: repositoryRoot,
		env: goEnvironment(),
		stdin: "inherit",
		stdout: "inherit",
		stderr: "inherit",
	});
	if (result.exitCode !== 0) process.exit(result.exitCode || 1);
}

export function checkGoFormat(executable = resolveGo()): void {
	const files: string[] = [];
	function walk(directory: string): void {
		for (const entry of readdirSync(directory, { withFileTypes: true })) {
			const path = join(directory, entry.name);
			if (entry.isDirectory()) walk(path);
			else if (entry.name.endsWith(".go")) files.push(path);
		}
	}
	walk(join(repositoryRoot, "cmd"));
	walk(join(repositoryRoot, "internal"));
	for (const entry of readdirSync(repositoryRoot, { withFileTypes: true })) {
		if (entry.isFile() && entry.name.endsWith(".go")) files.push(join(repositoryRoot, entry.name));
	}
	const gofmt = join(dirname(executable), process.platform === "win32" ? "gofmt.exe" : "gofmt");
	const result = Bun.spawnSync([gofmt, "-l", ...files.sort()], {
		cwd: repositoryRoot,
		stdout: "pipe",
		stderr: "inherit",
	});
	if (result.exitCode !== 0) process.exit(result.exitCode || 1);
	if (result.stdout.length !== 0) {
		console.error(`Go source needs gofmt:\n${result.stdout.toString().trimEnd()}`);
		process.exit(1);
	}
}

if (import.meta.main) {
	try {
		const args = process.argv.slice(2);
		if (args[0] === "fmt-check") checkGoFormat();
		else if (args.length > 0) runGo(args);
		else console.log(resolveGo());
	} catch (error) {
		console.error(error instanceof Error ? error.message : String(error));
		process.exit(1);
	}
}
