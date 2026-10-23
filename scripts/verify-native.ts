#!/usr/bin/env bun
/** Supported native checks, including the frozen Bun compatibility oracle. */
import { checkGoFormat, GO_VERSION, repositoryRoot, resolveGo, runGo } from "./go-tool.ts";

try {
	if (Bun.version !== "1.4.3")
		throw new Error(`Verification requires Bun 1.4.3; found ${Bun.version}.`);
	const go = resolveGo();
	console.log(`Checking native Atlas with Go ${GO_VERSION} and Bun ${Bun.version}`);
	checkGoFormat(go);
	const build = Bun.spawnSync([process.execPath, "scripts/build-native.ts"], {
		cwd: repositoryRoot,
		stdin: "inherit",
		stdout: "inherit",
		stderr: "inherit",
	});
	if (build.exitCode !== 0) process.exit(build.exitCode || 1);
	runGo(["vet", "-mod=vendor", "./..."], go);
	// A single package contains the integration suite. Serial package execution
	// lets Go stream each result instead of buffering it until that suite exits.
	runGo(["test", "-p=1", "-v", "-mod=vendor", "./..."], go);
} catch (error) {
	console.error(error instanceof Error ? error.message : String(error));
	process.exit(1);
}
