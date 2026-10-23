// Development-only compatibility capture. Run before replacing the TS entry point.
import { readdir } from "node:fs/promises";
import { resolve, basename } from "node:path";
import { Command, Option } from "commander";
import { configSchema } from "../compat/reference/src/schemas/config-schema.ts";
import { recordSchema } from "../compat/reference/src/schemas/record-schema.ts";

const root = resolve(import.meta.dir, "..");
const referenceRoot = resolve(root, "compat/reference");
const program = new Command().name("luuna-atlas").description("Structured expertise management")
  .version("0.11.0", "-v, --version", "Print version")
  .option("--json", "Output as structured JSON")
  .option("-q, --quiet", "Suppress non-error output")
  .option("--verbose", "Show full details in output")
  .option("--timing", "Print execution time to stderr")
  .option("--allow-unknown-types", "tolerate on-disk records of unregistered types (worktree/CI lag escape hatch)")
  .option("--allow-domain-mismatch", "tolerate records that violate per-domain allowed_types/required_fields (escape hatch for record/validate; ignored by sync)")
  .addOption(new Option("--format <format>", "output format for record-rendering commands (prime, query, search)").choices(["markdown","compact","xml","plain"]));

const order = ["init","add","record","edit","query","setup","prime","hook","onboard","status","validate","prune","archive","restore","search","show","stale","rank","outcome","doctor","ready","sync","delete","delete-domain","move","learn","compact","config","diff","update","upgrade","completions","audit"];
for (const name of order) {
  const exports = await import(resolve(referenceRoot,"src/commands",`${name}.ts`));
  const register = Object.entries(exports).find(([key])=>key.startsWith("register"))?.[1] as ((p:Command)=>void)|undefined;
  if (!register) throw new Error(`No command registrar for ${name}`);
  register(program);
}
const help = (args:string[]) => {
  const proc = Bun.spawnSync([process.execPath,resolve(referenceRoot,"src/cli.ts"),...args,"--help"],{cwd:root,stdout:"pipe",stderr:"pipe"});
  if(proc.exitCode!==0)throw new Error(proc.stderr.toString());
  return proc.stdout.toString().replaceAll("\r\n","\n");
};
const describe = (command:Command,path:string[]):unknown => ({
  name: command.name(), description: command.description(),
  hidden: (command as unknown as {_hidden:boolean})._hidden??false,
  arguments:command.registeredArguments.map(arg=>({name:arg.name(),required:arg.required,variadic:arg.variadic})),
  options:command.options.map(opt=>({flags:opt.flags,short:opt.short,long:opt.long,name:opt.name(),attribute:opt.attributeName(),description:opt.description,required:opt.required,optional:opt.optional,variadic:opt.variadic,negate:opt.negate,mandatory:opt.mandatory,default:opt.defaultValue,choices:opt.argChoices})),
  help:help(path), commands:command.commands.map(child=>describe(child,[...path,child.name()]))
});
const captured = {version:"0.11.0",baseline:"27477c313a3049b1579dfdc32c39da600726721f",cli:describe(program,[]),configSchema,recordSchema};
await Bun.write(resolve(root,"internal/atlas/contract.json"),JSON.stringify(captured,null,2)+"\n");
console.log(`Captured ${program.commands.length} top-level commands into internal/atlas/contract.json`);
