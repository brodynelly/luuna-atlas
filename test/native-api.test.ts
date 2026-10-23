import { afterEach, beforeEach, expect, test } from "bun:test";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
	appendOutcome,
	appendRecord,
	type ConventionRecord,
	editRecord,
	findDuplicate,
	generateRecordId,
	recordExpertise,
	searchRecords,
	sortByConfirmationScore,
	writeExpertiseFile,
} from "../src/index.ts";

let directory: string;
beforeEach(async () => {
	directory = await mkdtemp(join(tmpdir(), "atlas-native-api-"));
	await mkdir(join(directory, ".luuna/atlas/expertise"), { recursive: true });
	await writeFile(
		join(directory, ".luuna/atlas/config.yaml"),
		"version: '1'\ndomains:\n  testing: {}\n",
	);
});
afterEach(async () => {
	await rm(directory, { recursive: true, force: true });
});
const record = (content: string): ConventionRecord => ({
	type: "convention",
	content,
	classification: "foundational",
	recorded_at: "2026-01-01T00:00:00.000Z",
});

test("native facade retains input object identity and ID mutations", async () => {
	const first = record("Always preserve reference identity");
	const created = await recordExpertise("testing", first, { cwd: directory });
	expect(created.record).toBe(first);
	expect(first.id).toBe(generateRecordId(first));
	const duplicate = findDuplicate([first], record(first.content));
	expect(duplicate?.record).toBe(first);
	const matches = searchRecords([first], "identity");
	expect(matches[0]).toBe(first);
	expect(sortByConfirmationScore([first])[0]).toBe(first);
});

test("native facade retains edit arrays and appended outcome identity", async () => {
	const first = record("Always use typed outcomes");
	await recordExpertise("testing", first, { cwd: directory });
	const tags = ["typed"];
	const edited = await editRecord("testing", first.id as string, { tags }, { cwd: directory });
	expect(edited.tags).toBe(tags);
	const appended = await appendOutcome(
		"testing",
		first.id as string,
		{ status: "success" },
		{ cwd: directory },
	);
	expect(appended.record.outcomes?.at(-1)).toBe(appended.outcome);
});

test("native facade preserves ID assignment before failed file writes", async () => {
	const first = record("Always assign IDs before attempting file writes");
	await expect(appendRecord(join(directory, "missing", "append.jsonl"), first)).rejects.toThrow();
	expect(first.id).toBe(generateRecordId(first));
	const second = record("Always preserve failed rewrite ID assignment");
	await expect(
		writeExpertiseFile(join(directory, "missing", "rewrite.jsonl"), [second]),
	).rejects.toThrow();
	expect(second.id).toBe(generateRecordId(second));
});

test("native facade serializes concurrent public API mutations through file locks", async () => {
	const records = Array.from({ length: 12 }, (_, i) =>
		record(`Always preserve concurrent record ${i}`),
	);
	let eventLoopAdvanced = false;
	setTimeout(() => {
		eventLoopAdvanced = true;
	}, 0);
	const pending = records.map((item) => recordExpertise("testing", item, { cwd: directory }));
	const results = await Promise.all(pending);
	expect(eventLoopAdvanced).toBe(true);
	expect(
		results.every((result, i) => result.action === "created" && result.record === records[i]),
	).toBe(true);
	const { queryDomain } = await import("../src/index.ts");
	const persisted = await queryDomain("testing", { cwd: directory });
	expect(persisted).toHaveLength(12);
	expect(new Set(persisted.map((item) => item.id)).size).toBe(12);
});

test("native public reader preserves array rows accepted by the original reader", async () => {
	const path = join(directory, "array-rows.jsonl");
	await writeFile(path, '[]\n[{"nested":true}]\n');
	const { readExpertiseFile } = await import("../src/index.ts");
	const { readExpertiseFile: referenceReader } = await import(
		"../compat/reference/src/utils/expertise.ts"
	);
	expect(await readExpertiseFile(path)).toEqual(await referenceReader(path));
});

test("native config reads consume mutations to the exported defaults object", async () => {
	const { DEFAULT_CONFIG, readConfig } = await import("../src/index.ts");
	const oldMax = DEFAULT_CONFIG.governance.max_entries;
	const oldShelf = DEFAULT_CONFIG.classification_defaults.shelf_life.tactical;
	try {
		DEFAULT_CONFIG.governance.max_entries = 314;
		DEFAULT_CONFIG.classification_defaults.shelf_life.tactical = 27;
		const config = await readConfig(directory);
		expect(config.governance.max_entries).toBe(314);
		expect(config.classification_defaults.shelf_life.tactical).toBe(27);
	} finally {
		DEFAULT_CONFIG.governance.max_entries = oldMax;
		DEFAULT_CONFIG.classification_defaults.shelf_life.tactical = oldShelf;
	}
});

test("native API matches original Unicode duplicate, tag, and file case behavior", async () => {
	const source = await import("../compat/reference/src/index.ts");
	const native = await import("../src/index.ts");
	const greek = record("ΟΣ");
	const turkish = record("İstanbul");
	expect(native.findDuplicate([greek], record("ος"))?.index).toBe(
		source.findDuplicate([greek], record("ος"))?.index,
	);
	expect(native.findDuplicate([turkish], record("i stanbul"))?.index).toBe(
		source.findDuplicate([turkish], record("i stanbul"))?.index,
	);
	await native.recordExpertise(
		"testing",
		{ ...record("Unicode tags"), tags: ["ΟΣ"] },
		{ cwd: directory },
	);
	expect(await native.searchExpertise("Unicode", { tag: "ος", cwd: directory })).toEqual(
		await source.searchExpertise("Unicode", { tag: "ος", cwd: directory }),
	);
	const pattern = {
		type: "pattern" as const,
		name: "Unicode file",
		description: "Unicode file matching",
		files: ["İSTANBUL.ts"],
		classification: "foundational" as const,
		recorded_at: "2026-01-01T00:00:00.000Z",
	};
	await native.recordExpertise("testing", pattern, { cwd: directory });
	expect(await native.queryDomain("testing", { file: "i̇stanbul", cwd: directory })).toEqual(
		await source.queryDomain("testing", { file: "i̇stanbul", cwd: directory }),
	);
	expect(native.searchRecords([turkish], "i")).toEqual(source.searchRecords([turkish], "i"));
});

test("native scoring preserves NaN, infinities, and signed zero through private IPC", async () => {
	const native = await import("../src/index.ts");
	const source = await import("../compat/reference/src/index.ts");
	for (const base of [Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY, -0, 0]) {
		for (const outcomes of [undefined, [{ status: "success" as const }]]) {
			const item = { ...record("Numeric scoring"), outcomes };
			for (const factor of [
				undefined,
				Number.NaN,
				Number.POSITIVE_INFINITY,
				Number.NEGATIVE_INFINITY,
				-0,
				0.1,
			]) {
				const expected = source.applyConfirmationBoost(base, item, factor);
				expect(Object.is(native.applyConfirmationBoost(base, item, factor), expected)).toBe(true);
			}
		}
	}
	const old = native.DEFAULT_CONFIG.governance.max_entries;
	try {
		native.DEFAULT_CONFIG.governance.max_entries = Number.NaN;
		expect(Number.isNaN((await native.readConfig(directory)).governance.max_entries)).toBe(true);
	} finally {
		native.DEFAULT_CONFIG.governance.max_entries = old;
	}
});
