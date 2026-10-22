import { LEGACY_MARKER_END, LEGACY_MARKER_START } from "./legacy-layout.ts";

const MARKER_START = "<!-- luuna-atlas:start -->";
const MARKER_END = "<!-- luuna-atlas:end -->";

export { MARKER_START, MARKER_END };

// Sections written by older versions are still recognised so they are
// replaced or removed in place instead of being duplicated.
const PAIRS: ReadonlyArray<readonly [string, string]> = [
	[MARKER_START, MARKER_END],
	[LEGACY_MARKER_START, LEGACY_MARKER_END],
];

function findPair(content: string): readonly [string, string, number, number] | null {
	for (const [start, end] of PAIRS) {
		const startIdx = content.indexOf(start);
		const endIdx = content.indexOf(end);
		if (startIdx !== -1 && endIdx !== -1) return [start, end, startIdx, endIdx];
	}
	return null;
}

/**
 * Check whether content contains the luuna-atlas marker section.
 */
export function hasMarkerSection(content: string): boolean {
	return PAIRS.some(([start]) => content.includes(start));
}

/**
 * Replace the marker-bounded section with new content.
 * Returns null if no markers found.
 */
export function replaceMarkerSection(content: string, newSection: string): string | null {
	const pair = findPair(content);
	if (!pair) return null;
	const [, end, startIdx, endIdx] = pair;
	return content.substring(0, startIdx) + newSection + content.substring(endIdx + end.length);
}

/**
 * Remove the marker-bounded section entirely.
 * Cleans up extra newlines left behind.
 */
export function removeMarkerSection(content: string): string {
	const pair = findPair(content);
	if (!pair) return content;
	const [, end, startIdx, endIdx] = pair;
	const before = content.substring(0, startIdx);
	const after = content.substring(endIdx + end.length);

	return `${(before + after).replace(/\n{3,}/g, "\n\n").trim()}\n`;
}

/**
 * Wrap a snippet in luuna-atlas markers.
 */
export function wrapInMarkers(snippet: string): string {
	return `${MARKER_START}\n${snippet}${MARKER_END}`;
}
