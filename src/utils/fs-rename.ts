import { rename } from "node:fs/promises";
import { setTimeout as sleep } from "node:timers/promises";

// Codes Windows returns while the rename target is held open by a reader, an
// antivirus or indexer scan, or another rename that is replacing it.
const TRANSIENT_CODES = new Set(["EPERM", "EACCES", "EBUSY"]);
const MAX_ATTEMPTS = 20;

/**
 * Rename `from` over `to`. POSIX replaces the target atomically even while it
 * is open, so this is a plain rename there. Windows refuses to replace a file
 * another handle holds open, so retry with a short backoff (about 1.7s in
 * total) before surfacing the error.
 */
export async function renameReplacing(from: string, to: string): Promise<void> {
	for (let attempt = 1; ; attempt++) {
		try {
			await rename(from, to);
			return;
		} catch (err) {
			const code = (err as NodeJS.ErrnoException).code;
			if (
				process.platform !== "win32" ||
				attempt >= MAX_ATTEMPTS ||
				code === undefined ||
				!TRANSIENT_CODES.has(code)
			) {
				throw err;
			}
			await sleep(Math.min(5 * 2 ** attempt, 100));
		}
	}
}
