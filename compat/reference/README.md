# Frozen compatibility reference

This is the TypeScript/Bun implementation from Atlas commit `27477c313a3049b1579dfdc32c39da600726721f`, version `0.11.0`. It is a development oracle for the Go rewrite. `integrity.json` records SHA-256 hashes of its original source files and package metadata after normalizing Git checkout line endings to LF; the native test suite checks those hashes.

Production commands and library operations run in Go. They do not import or execute this reference. The production package file list excludes `compat/`. Existing internal Bun tests import this directory explicitly; subprocess CLI tests and public API tests use the Go compatibility transports in the repository's current `src/`.

Do not modify the oracle to make native tests pass. Differential tests use disposable projects, compare command outputs and persisted state, and normalize generated timestamps and project paths. Hook-failure fixtures silence nondeterministic process-log headers while checking CLI errors, warnings and data effects. Legacy uncaught Bun source stack frames are runtime-specific: the full underlying error, exit status, stdout and unchanged data remain checked.

The original MIT notice and provenance remain in the repository's `LICENSE` and `UPSTREAM.md`.
