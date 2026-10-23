/** Public TypeScript compatibility facade for the native Go library. */
import { nativeCallAsync } from "./native.ts";
import type { Classification, ExpertiseRecord, Outcome } from "./types.ts";

export interface RecordOptions {
	force?: boolean;
	cwd?: string;
}

export interface RecordResult {
	action: "created" | "updated" | "skipped";
	record: ExpertiseRecord;
}

export interface SearchOptions {
	domain?: string;
	type?: string;
	tag?: string;
	classification?: string;
	file?: string;
	cwd?: string;
}

export interface SearchResult {
	domain: string;
	records: ExpertiseRecord[];
}

export interface QueryOptions {
	type?: string;
	classification?: string;
	file?: string;
	cwd?: string;
}

export interface EditOptions {
	cwd?: string;
}

export interface OutcomeOptions {
	cwd?: string;
}

export interface AppendOutcomeResult {
	record: ExpertiseRecord;
	outcome: Outcome;
	total_outcomes: number;
}

export interface RecordUpdates {
	classification?: Classification;
	tags?: string[];
	relates_to?: string[];
	supersedes?: string[];
	outcomes?: Outcome[];
	// type-specific fields
	content?: string;
	name?: string;
	description?: string;
	resolution?: string;
	title?: string;
	rationale?: string;
	files?: string[];
}

export async function recordExpertise(
	domain: string,
	record: ExpertiseRecord,
	options: RecordOptions = {},
): Promise<RecordResult> {
	const response = await nativeCallAsync<{
		result: RecordResult;
		mutation: ExpertiseRecord;
		error?: string;
	}>("recordExpertiseTransport", [domain, record, options]);
	if (response.mutation.id !== undefined) record.id = response.mutation.id;
	if (response.error !== undefined) throw new Error(response.error);
	const result = response.result;
	if (result.action !== "skipped") {
		if (result.record.id !== undefined) record.id = result.record.id;
		result.record = record;
	}
	return result;
}

export async function searchExpertise(
	query: string,
	options: SearchOptions = {},
): Promise<SearchResult[]> {
	return await nativeCallAsync<SearchResult[]>("searchExpertise", [query, options]);
}

export async function queryDomain(
	domain: string,
	options: QueryOptions = {},
): Promise<ExpertiseRecord[]> {
	return await nativeCallAsync<ExpertiseRecord[]>("queryDomain", [domain, options]);
}

export async function editRecord(
	domain: string,
	id: string,
	updates: RecordUpdates,
	options: EditOptions = {},
): Promise<ExpertiseRecord> {
	const record = await nativeCallAsync<ExpertiseRecord>("editRecord", [
		domain,
		id,
		updates,
		options,
	]);
	// The source API assigns supplied arrays by reference; IPC must restore those references.
	if (updates.tags !== undefined) record.tags = updates.tags;
	if (updates.relates_to !== undefined) record.relates_to = updates.relates_to;
	if (updates.supersedes !== undefined) record.supersedes = updates.supersedes;
	if (updates.outcomes !== undefined) record.outcomes = updates.outcomes;
	if (updates.files !== undefined && (record.type === "pattern" || record.type === "reference")) {
		record.files = updates.files;
	}
	return record;
}

export async function appendOutcome(
	domain: string,
	id: string,
	outcome: Outcome,
	options: OutcomeOptions = {},
): Promise<AppendOutcomeResult> {
	const result = await nativeCallAsync<AppendOutcomeResult>("appendOutcome", [
		domain,
		id,
		outcome,
		options,
	]);
	const outcomes = result.record.outcomes;
	if (outcomes?.length) outcomes[outcomes.length - 1] = result.outcome;
	return result;
}
