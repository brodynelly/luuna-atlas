import { copyFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { resolveNative } from "../src/native.ts";
import { resolveGo } from "./go-tool.ts";

// Check the pinned development toolchain even when an existing build is cached.
resolveGo();
// Building the checkout must not overwrite aliases beside a transport override.
const override = process.env.LUUNA_ATLAS_BINARY;
delete process.env.LUUNA_ATLAS_BINARY;
let path: string;
try {
	path = resolveNative();
} finally {
	if (override === undefined) delete process.env.LUUNA_ATLAS_BINARY;
	else process.env.LUUNA_ATLAS_BINARY = override;
}
for (const alias of ["lat", "la"])
	copyFileSync(path, join(dirname(path), `${alias}${process.platform === "win32" ? ".exe" : ""}`));
console.log(`Built native Atlas and aliases: ${path}`);
