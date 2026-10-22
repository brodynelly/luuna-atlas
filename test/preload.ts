// Test preload (bunfig.toml [test].preload). On Windows, put Git for Windows'
// POSIX tools (sh, cat, sleep, ...) first on PATH: hook and shell-recipe
// tests run real `sh` scripts, and from PowerShell or cmd `sh` is usually not
// on PATH at all. Git ships the tools under <git root>/usr/bin, three levels
// above `git --exec-path` (<git root>/mingw64/libexec/git-core).

import { existsSync } from "node:fs";
import { delimiter, join, resolve } from "node:path";

if (process.platform === "win32") {
	const result = Bun.spawnSync(["git", "--exec-path"], { stdout: "pipe", stderr: "ignore" });
	const execPath = result.exitCode === 0 ? result.stdout.toString().trim() : "";
	const usrBin = execPath ? resolve(execPath, "..", "..", "..", "usr", "bin") : "";
	if (usrBin && existsSync(join(usrBin, "sh.exe"))) {
		process.env.PATH = `${usrBin}${delimiter}${process.env.PATH ?? ""}`;
	}
}
