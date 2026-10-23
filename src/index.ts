/** Public compatibility surface. All Atlas algorithms and storage run in Go. */
import { nativeCall, nativeCallAsync, setNativeDefaults } from "./native.ts";
import type { ConfigSchema, RecordSchema } from "./schema-types.ts";
import type { AtlasConfig, ExpertiseRecord, ScoredRecord } from "./types.ts";

export type {
	AppendOutcomeResult,
	EditOptions,
	OutcomeOptions,
	QueryOptions,
	RecordOptions,
	RecordResult,
	RecordUpdates,
	SearchOptions,
	SearchResult,
} from "./api.ts";
export { appendOutcome, editRecord, queryDomain, recordExpertise, searchExpertise } from "./api.ts";
export type {
	AtlasConfig,
	Classification,
	ConventionRecord,
	DecisionRecord,
	Evidence,
	ExpertiseRecord,
	FailureRecord,
	GuideRecord,
	Outcome,
	PatternRecord,
	RecordType,
	ReferenceRecord,
	ScoredRecord,
} from "./types.ts";

export const configSchema = nativeCall<ConfigSchema>("configSchema", []);
export const recordSchema = nativeCall<RecordSchema>("recordSchema", []);
export const DEFAULT_CONFIG = nativeCall<AtlasConfig>("defaultConfig", []);
setNativeDefaults(DEFAULT_CONFIG);

export async function readConfig(cwd?: string): Promise<AtlasConfig> {
	return nativeCallAsync<AtlasConfig>("readConfig", [cwd]);
}
export function getExpertisePath(domain: string, cwd?: string): string {
	return nativeCall<string>("getExpertisePath", [domain, cwd]);
}
export async function appendRecord(path: string, record: ExpertiseRecord): Promise<void> {
	const response = await nativeCallAsync<{ mutation: ExpertiseRecord; error?: string }>(
		"appendRecordTransport",
		[path, record],
	);
	if (response.mutation.id !== undefined) record.id = response.mutation.id;
	if (response.error !== undefined) throw new Error(response.error);
}
export async function readExpertiseFile(
	path: string,
	options: { allowUnknownTypes?: boolean } = {},
): Promise<ExpertiseRecord[]> {
	return nativeCallAsync<ExpertiseRecord[]>("readExpertiseFile", [path, options]);
}
export async function writeExpertiseFile(path: string, records: ExpertiseRecord[]): Promise<void> {
	const response = await nativeCallAsync<{ mutation: ExpertiseRecord[]; error?: string }>(
		"writeExpertiseFileTransport",
		[path, records],
	);
	const updated = response.mutation;
	for (const [i, record] of records.entries()) {
		const id = updated[i]?.id;
		if (id !== undefined) record.id = id;
	}
	if (response.error !== undefined) throw new Error(response.error);
}
export function findDuplicate(
	existing: ExpertiseRecord[],
	record: ExpertiseRecord,
): { index: number; record: ExpertiseRecord } | null {
	const duplicate = nativeCall<{ index: number } | null>("findDuplicate", [existing, record]);
	if (!duplicate) return null;
	const original = existing[duplicate.index];
	return original ? { index: duplicate.index, record: original } : null;
}
export function generateRecordId(record: ExpertiseRecord): string {
	return nativeCall<string>("generateRecordId", [record]);
}
export function searchRecords(
	records: ExpertiseRecord[],
	query: string,
	options: { boostFactor?: number } = {},
): ExpertiseRecord[] {
	const indexes = nativeCall<number[]>("searchRecordIndexes", [records, query, options]);
	return indexes.map((index) => records[index] as ExpertiseRecord);
}
export function getSuccessCount(record: ScoredRecord): number {
	return nativeCall<number>("getSuccessCount", [record]);
}
export function getFailureCount(record: ScoredRecord): number {
	return nativeCall<number>("getFailureCount", [record]);
}
export function getTotalApplications(record: ScoredRecord): number {
	return nativeCall<number>("getTotalApplications", [record]);
}
export function getSuccessRate(record: ScoredRecord): number {
	return nativeCall<number>("getSuccessRate", [record]);
}
export function computeConfirmationScore(record: ScoredRecord): number {
	return nativeCall<number>("computeConfirmationScore", [record]);
}
export function applyConfirmationBoost(
	baseScore: number,
	record: ScoredRecord,
	boostFactor = 0.1,
): number {
	return nativeCall<number>("applyConfirmationBoost", [baseScore, record, boostFactor]);
}
export function sortByConfirmationScore<T extends ScoredRecord>(records: T[]): T[] {
	const indexes = nativeCall<number[]>("sortByConfirmationScoreIndexes", [records]);
	return indexes.map((index) => records[index] as T);
}
