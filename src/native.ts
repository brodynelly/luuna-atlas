/** Compatibility transport only. Atlas command and library behavior lives in Go. */
import { createHash } from "node:crypto";
import {
	existsSync,
	mkdirSync,
	readdirSync,
	readFileSync,
	renameSync,
	rmSync,
	statSync,
	writeFileSync,
} from "node:fs";
import { join, resolve } from "node:path";

const root = resolve(import.meta.dir, "..");
const binaryName = process.platform === "win32" ? "luuna-atlas.exe" : "luuna-atlas";
const output = join(root, ".atlas-bin", binaryName);
let nativeDefaults: unknown;

/** Keep the original mutable DEFAULT_CONFIG export connected to native reads. */
export function setNativeDefaults(defaults: unknown): void {
	nativeDefaults = defaults;
}

type NumericToken = { path: (string | number)[]; value: string };

function serializeRequest(operation: string, args: unknown[]): Buffer {
	const numbers: NumericToken[] = [];
	const paths = new WeakMap<object, (string | number)[]>();
	const json = JSON.stringify({ operation, args, defaults: nativeDefaults }, function (key, value) {
		const parent = paths.get(this);
		const path = parent ? [...parent, Array.isArray(this) ? Number(key) : key] : [];
		if (typeof value === "number" && (!Number.isFinite(value) || Object.is(value, -0))) {
			numbers.push({ path, value: Object.is(value, -0) ? "-0" : String(value) });
		}
		if (value !== null && typeof value === "object") paths.set(value, path);
		return value;
	});
	return Buffer.from(`${json.slice(0, -1)},"numbers":${JSON.stringify(numbers)}}`);
}

function restoreNumbers<T>(value: T, numbers: NumericToken[] = []): T {
	let result = value;
	for (const token of numbers) {
		const restored = token.value === "-0" ? -0 : Number(token.value);
		if (token.path.length === 0) {
			result = restored as T;
			continue;
		}
		let parent: unknown = result;
		for (const key of token.path.slice(0, -1))
			parent = (parent as Record<string | number, unknown>)[key];
		const key = token.path.at(-1) as string | number;
		(parent as Record<string | number, unknown>)[key] = restored;
	}
	return result;
}

function goExecutable(): string {
	const candidates = [
		process.env.LUUNA_ATLAS_GO,
		Bun.which("go"),
		process.platform === "win32" ? "C:\\Program Files\\Go\\bin\\go.exe" : undefined,
	];
	for (const candidate of candidates) if (candidate && existsSync(candidate)) return candidate;
	throw new Error(
		"Building Luuna Atlas requires Go 1.27.0 or newer. Install Go or use a prebuilt luuna-atlas executable.",
	);
}

function sourceDigest(): string {
	const hash = createHash("sha256");
	hash.update(process.platform).update(process.arch);
	const walk = (directory: string): void => {
		for (const entry of readdirSync(directory, { withFileTypes: true }).sort((a, b) =>
			a.name.localeCompare(b.name),
		)) {
			const path = join(directory, entry.name);
			if (entry.isDirectory()) walk(path);
			else if (
				(entry.name.endsWith(".go") && !entry.name.endsWith("_test.go")) ||
				entry.name.endsWith(".json")
			)
				hash.update(path.slice(root.length)).update(readFileSync(path));
		}
	};
	for (const name of ["go.mod", "go.sum", "vendor/modules.txt"])
		hash.update(readFileSync(join(root, name)));
	walk(join(root, "cmd"));
	walk(join(root, "internal"));
	walk(join(root, "vendor"));
	return hash.digest("hex");
}

function staleBuildLock(lock: string): boolean {
	try {
		if (Date.now() - statSync(lock).mtimeMs <= 120_000) return false;
		rmSync(lock, { recursive: true, force: true });
		return true;
	} catch (error) {
		if ((error as NodeJS.ErrnoException).code === "ENOENT") return true;
		throw error;
	}
}

function acquireBuildLock(lock: string, current: () => boolean): boolean {
	const deadline = Date.now() + 120_000;
	for (;;) {
		try {
			mkdirSync(lock);
			return true;
		} catch (error) {
			if ((error as NodeJS.ErrnoException).code !== "EEXIST") throw error;
			if (current()) return false;
			if (staleBuildLock(lock)) continue;
			if (Date.now() > deadline) throw new Error("Timed out waiting for the native Atlas build.");
			Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 50);
		}
	}
}

export function resolveNative(): string {
	if (process.env.LUUNA_ATLAS_BINARY) return resolve(process.env.LUUNA_ATLAS_BINARY);
	const digest = sourceDigest();
	const stamp = `${output}.source`;
	const current = (): boolean =>
		existsSync(output) && existsSync(stamp) && readFileSync(stamp, "utf8").trim() === digest;
	if (current()) return output;
	mkdirSync(join(root, ".atlas-bin"), { recursive: true });
	const lock = `${output}.build-lock`;
	if (!acquireBuildLock(lock, current)) return output;
	const temporary = `${output}.tmp-${process.pid}${process.platform === "win32" ? ".exe" : ""}`;
	try {
		if (current()) return output;
		const built = Bun.spawnSync(
			[goExecutable(), "build", "-mod=vendor", "-trimpath", "-o", temporary, "./cmd/luuna-atlas"],
			{
				cwd: root,
				stdout: "pipe",
				stderr: "pipe",
				env: {
					...process.env,
					GOTOOLCHAIN: "local",
					GOPROXY: "off",
					GOSUMDB: "off",
					GOOS: process.platform === "win32" ? "windows" : process.platform,
					GOARCH: process.arch === "x64" ? "amd64" : process.arch === "ia32" ? "386" : process.arch,
					CGO_ENABLED: "0",
				},
			},
		);
		if (built.exitCode !== 0)
			throw new Error(built.stderr.toString().trim() || "Native Atlas build failed.");
		renameSync(temporary, output);
		writeFileSync(stamp, `${digest}\n`);
		return output;
	} finally {
		rmSync(temporary, { force: true });
		rmSync(lock, { recursive: true, force: true });
	}
}

export function nativeCall<T>(operation: string, args: unknown[]): T {
	const result = Bun.spawnSync([resolveNative()], {
		stdin: serializeRequest(operation, args),
		stdout: "pipe",
		stderr: "pipe",
		env: { ...process.env, LUUNA_ATLAS_API_BRIDGE: "1" },
	});
	let response: { success: boolean; result?: T; error?: string; numbers?: NumericToken[] };
	try {
		response = JSON.parse(result.stdout.toString());
	} catch {
		throw new Error(result.stderr.toString().trim() || "Native Atlas API returned invalid JSON.");
	}
	if (!response.success) throw new Error(response.error || "Native Atlas API failed.");
	return restoreNumbers(response.result as T, response.numbers);
}

export async function nativeCallAsync<T>(operation: string, args: unknown[]): Promise<T> {
	const child = Bun.spawn([resolveNative()], {
		stdin: serializeRequest(operation, args),
		stdout: "pipe",
		stderr: "pipe",
		env: { ...process.env, LUUNA_ATLAS_API_BRIDGE: "1" },
	});
	const [stdout, stderr] = await Promise.all([
		new Response(child.stdout).text(),
		new Response(child.stderr).text(),
		child.exited,
	]);
	let response: { success: boolean; result?: T; error?: string; numbers?: NumericToken[] };
	try {
		response = JSON.parse(stdout);
	} catch {
		throw new Error(stderr.trim() || "Native Atlas API returned invalid JSON.");
	}
	if (!response.success) throw new Error(response.error || "Native Atlas API failed.");
	return restoreNumbers(response.result as T, response.numbers);
}
