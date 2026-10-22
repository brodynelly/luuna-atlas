import { existsSync } from "node:fs";
import type { Command } from "commander";
import { getAtlasDir, initAtlasDir } from "../utils/config.ts";
import { outputJson } from "../utils/json-output.ts";
import { brand, isQuiet } from "../utils/palette.ts";

export function registerInitCommand(program: Command): void {
	program
		.command("init")
		.description("Initialize .luuna/atlas/ in the current project")
		.action(async () => {
			const jsonMode = program.opts().json === true;
			const atlasDir = getAtlasDir();
			const alreadyExists = existsSync(atlasDir);

			await initAtlasDir();

			if (jsonMode) {
				outputJson({
					success: true,
					command: "init",
					created: !alreadyExists,
					path: atlasDir,
				});
			} else if (alreadyExists) {
				if (!isQuiet())
					console.log(brand("Updated .luuna/atlas/ — filled in any missing artifacts."));
			} else {
				if (!isQuiet()) console.log(brand(`Initialized .luuna/atlas/ in ${process.cwd()}`));
			}
		});
}
