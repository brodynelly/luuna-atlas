#!/usr/bin/env bun
// Existing source/Bun launchers remain compatible; all CLI behavior runs in Go.
import { resolveNative } from "./native.ts";
export const VERSION = "0.11.0";
try {
	const child = Bun.spawn([resolveNative(), ...process.argv.slice(2)], {
		stdin: "inherit",
		stdout: "inherit",
		stderr: "inherit",
		env: process.env,
	});
	process.exitCode = await child.exited;
} catch (error) {
	console.error(`Error: ${error instanceof Error ? error.message : String(error)}`);
	process.exitCode = 1;
}
