import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { mkdtemp, open, readdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { renameReplacing } from "../../src/utils/fs-rename.ts";

describe("renameReplacing", () => {
	let tmpDir: string;

	beforeEach(async () => {
		tmpDir = await mkdtemp(join(tmpdir(), "mulch-fs-rename-test-"));
	});

	afterEach(async () => {
		await rm(tmpDir, { recursive: true, force: true });
	});

	it("replaces the target and removes the source", async () => {
		const target = join(tmpDir, "records.jsonl");
		const source = join(tmpDir, "records.jsonl.tmp");
		await writeFile(target, "old\n", "utf-8");
		await writeFile(source, "new\n", "utf-8");

		await renameReplacing(source, target);

		expect(await readFile(target, "utf-8")).toBe("new\n");
		expect(await readdir(tmpDir)).toEqual(["records.jsonl"]);
	});

	it("replaces a target that a reader briefly holds open", async () => {
		// Windows rejects the rename while the handle is open; the retry must
		// outlast it. POSIX replaces the open file immediately.
		const target = join(tmpDir, "records.jsonl");
		const source = join(tmpDir, "records.jsonl.tmp");
		await writeFile(target, "old\n", "utf-8");
		await writeFile(source, "new\n", "utf-8");
		const reader = await open(target, "r");
		const closed = new Promise<void>((resolve) => {
			setTimeout(() => {
				void reader.close().then(resolve);
			}, 50);
		});

		await renameReplacing(source, target);
		await closed;

		expect(await readFile(target, "utf-8")).toBe("new\n");
	});

	it("surfaces errors that are not transient", async () => {
		await expect(
			renameReplacing(join(tmpDir, "missing.tmp"), join(tmpDir, "records.jsonl")),
		).rejects.toMatchObject({ code: "ENOENT" });
	});
});
