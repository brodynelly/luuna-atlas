// Legacy (pre-Luuna) layout compatibility. This is the ONLY module that may
// name the old storage directory, config file, sibling-tool directory,
// evidence key, command names, or marker strings. Everything here exists so
// projects created before the layout move keep working.
import { lstatSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { join } from "node:path";

/** Old project data directory (relative to the project root). */
const LEGACY_STORE_DIR = ".mulch";
/** Old config file name inside the store. */
const LEGACY_CONFIG_FILE = "mulch.config.yaml";
/** Old Missions data directory (read-only fallback). */
const LEGACY_MISSIONS_DIR = ".seeds";
/** Old evidence key naming a Missions issue; still accepted when reading. */
export const LEGACY_EVIDENCE_KEY = "seeds";

/** Old integration markers written into agent instruction files. */
export const LEGACY_MARKER_START = "<!-- mulch:start -->";
export const LEGACY_MARKER_END = "<!-- mulch:end -->";

/** Command names of Claude Code hook handlers installed by older versions. */
export const LEGACY_HOOK_COMMAND_RE = /^\s*(ml|mulch)\s+(prime|hook)(\s|$)/;
/** Bare command names the hook guard must keep treating as Atlas itself. */
export const LEGACY_COMMAND_NAMES: readonly string[] = ["ml", "mulch"];

export interface StoreLayout {
	/** Directory under the root that holds Luuna data, e.g. ".luuna". */
	luunaDir: string;
	/** Store path relative to the root, e.g. ".luuna/atlas". */
	storeRel: string;
	/** Config file name inside the store, e.g. "config.yaml". */
	configFile: string;
}

function lstatOrNull(path: string) {
	try {
		return lstatSync(path);
	} catch {
		return null;
	}
}

/** True when a legacy store directory exists at `root`. */
export function hasLegacyStore(root: string): boolean {
	const st = lstatOrNull(join(root, LEGACY_STORE_DIR));
	return st !== null && (st.isDirectory() || st.isSymbolicLink());
}

function rewriteLegacyLines(file: string, newRel: string): void {
	let text: string;
	try {
		text = readFileSync(file, "utf-8");
	} catch {
		return;
	}
	const re = new RegExp(`^(\\s*/?)${LEGACY_STORE_DIR.replace(".", "\\.")}(?=/)`, "gm");
	const next = text.replace(re, `$1${newRel}`);
	if (next !== text) writeFileSync(file, next, "utf-8");
}

/**
 * Move a legacy store into the Luuna layout. Returns true when a move
 * happened. Throws when the state is ambiguous or unsafe. The single notice
 * line goes to stderr only; stdout carries machine-readable output.
 */
export function migrateLegacyStore(root: string, layout: StoreLayout): boolean {
	const legacy = join(root, LEGACY_STORE_DIR);
	const legacySt = lstatOrNull(legacy);
	if (!legacySt) return false;

	const luunaPath = join(root, layout.luunaDir);
	const target = join(root, layout.storeRel);
	const luunaSt = lstatOrNull(luunaPath);

	if (legacySt.isSymbolicLink()) {
		throw new Error(
			`Refusing to migrate ${LEGACY_STORE_DIR}: it is a symbolic link. Move its contents to ${layout.storeRel} manually.`,
		);
	}
	if (!legacySt.isDirectory()) return false;
	if (luunaSt?.isSymbolicLink()) {
		throw new Error(
			`Refusing to migrate ${LEGACY_STORE_DIR}: ${layout.luunaDir} is a symbolic link. Replace it with a real directory first.`,
		);
	}
	if (lstatOrNull(target)) {
		throw new Error(
			`Both ${LEGACY_STORE_DIR}/ and ${layout.storeRel}/ exist. Resolve this by hand: keep ${layout.storeRel}/ and remove or archive ${LEGACY_STORE_DIR}/ (or the reverse), then retry.`,
		);
	}

	mkdirSync(luunaPath, { recursive: true });
	renameSync(legacy, target);
	const legacyConfig = join(target, LEGACY_CONFIG_FILE);
	if (lstatOrNull(legacyConfig) && !lstatOrNull(join(target, layout.configFile))) {
		renameSync(legacyConfig, join(target, layout.configFile));
	}
	rewriteLegacyLines(join(root, ".gitattributes"), layout.storeRel);
	rewriteLegacyLines(join(root, ".gitignore"), layout.storeRel);

	process.stderr.write(
		`luuna-atlas: moved legacy ${LEGACY_STORE_DIR}/ to ${layout.storeRel}/ (${LEGACY_CONFIG_FILE} is now ${layout.configFile})\n`,
	);
	return true;
}

/**
 * Pick the Missions data directory: the current one if present, otherwise the
 * legacy one if present (read-only fallback), otherwise the current one.
 */
export function resolveMissionsDir(cwd: string, currentRel: string): string {
	if (lstatOrNull(join(cwd, currentRel))) return currentRel;
	if (lstatOrNull(join(cwd, LEGACY_MISSIONS_DIR))) return LEGACY_MISSIONS_DIR;
	return currentRel;
}

/** Read the Missions issue id from an evidence object (current key first). */
export function evidenceMissionRef(ev: unknown): string | undefined {
	if (!ev || typeof ev !== "object") return undefined;
	const e = ev as Record<string, unknown>;
	const v = e.missions ?? e[LEGACY_EVIDENCE_KEY];
	return typeof v === "string" && v ? v : undefined;
}
