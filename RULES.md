# Autonomous Agent Rules

## Authority

The only source of truth for these rules is:

`/home/alex/Desktop/Private/openposterdb/openposterdb/RULES.md`

Before doing anything else:

1. Read `RULES.md` completely.
2. Follow its current contents exactly.
3. If `RULES.md` changes, immediately discard all previous rules and use the new version.
4. If `RULES.md` is empty, there are no active rules.

---

# Repository Boundary

The repository root is:

`/home/alex/Desktop/Private/openposterdb/openposterdb`

The agent must behave as though nothing exists outside this directory.

The agent must never:

* Read any file outside the repository.
* Search outside the repository.
* Enumerate directories outside the repository.
* Traverse above the repository root.
* Use relative paths (`..`) to leave the repository.

The following are explicitly forbidden:

* `/usr`
* `/bin`
* `/sbin`
* `/etc`
* `/opt`
* `/var`
* `/tmp`
* `/home`
* `$HOME`
* `~`
* any mounted filesystem outside the repository

The agent must never attempt to discover tools or files outside the repository.

If a required executable is unavailable, the agent must stop and report the problem instead of attempting to locate it.

---

# Allowed Operations

The only allowed actions are:

* Read repository files.
* Write permitted repository files.
* Execute `date`.
* Execute `go build`.
* Execute `go test`.
* Execute `go mod tidy`.
* Execute `cargo build`.
* Execute `cargo test`.
* Access the internet only for documentation and research.

No other commands are allowed.

Forbidden examples include (but are not limited to):

* `find`
* `which`
* `whereis`
* `locate`
* `ls`
* `tree`
* `pwd`
* `env`
* `printenv`
* `export`
* `cd` outside the repository
* shell scripts
* arbitrary executables
* package managers
* installers
* downloading binaries
* cloning repositories
* installing Go
* installing Rust
* modifying PATH
* modifying environment variables
* executing any program not explicitly listed above

If an allowed command cannot be executed successfully, the agent must stop and report the failure.

The agent must never attempt to repair its execution environment.

---

# Working Directory

All commands must execute from inside the repository.

The agent must never change into any directory outside the repository.

---

# Internet Usage

Internet access is allowed only for:

* Go documentation
* Rust documentation
* Standard library documentation
* Language references
* Library documentation
* Equivalent Go libraries
* Compiler errors
* Build errors

Internet access is **not** allowed for:

* downloading software
* downloading binaries
* package installers
* repository cloning
* code generation websites
* unrelated browsing

Research findings must be documented in `NOTES.md`.

Include:

* links
* reasoning
* decisions

---

# File Modification Rules

Allowed writes:

* any file in `api-go/`
* `NOTES.md`
* any file located directly in the repository root

Allowed:

* create Go files in `api-go/`
* modify existing Go files in `api-go/`
* modify root files
* update `go.mod`
* update `go.sum`

Forbidden:

* modifying anything inside `api/`
* deleting any file
* renaming files
* moving files

The `api/` directory is immutable.

---

# Initial Analysis Phase

Before writing any Go code the agent must:

1. Read every file in the repository.
2. Read every file completely.
3. Read every file regardless of extension.
4. Inspect binary files without external tools.
5. Understand project structure.
6. Understand migration requirements.
7. Determine a reading order.
8. Create `NOTES.md`.

No Go code may be written before analysis is complete.

---

# NOTES.md

`NOTES.md` is mandatory.

It must always exist.

The first section must be:

## Potential Issues

This section must contain:

* migration risks
* unclear behavior
* design concerns
* unresolved questions
* compatibility issues

The remainder of the document must contain:

* repository understanding
* file summaries
* reading progress
* migration order
* completed work
* remaining work
* internet research
* implementation decisions

The agent may reorganize or rewrite the notes at any time.

---

# Waiting State

After analysis completes the agent must stop.

It must wait until the user explicitly says:

* go
* begin
* start migration
* convert
* or another unmistakable equivalent

No Go code may be written before that instruction.

---

# Go Migration Rules

Once permission is given:

1. Write the migration workflow into `NOTES.md`.
2. Work on one Go file at a time.
3. Finish one file before another.
4. Preserve behavior exactly.
5. Do not redesign architecture.
6. Do not optimize behavior.
7. Do not change APIs unless required.
8. Existing `api-go/` files may be modified.
9. Root files may be modified as needed.

---

# Build Rules

Only these commands may be used:

* `go build`
* `go test`
* `go mod tidy`
* `cargo build`
* `cargo test`

The agent must never:

* search for Go
* search for Cargo
* modify PATH
* export environment variables
* install missing software
* execute helper scripts

If any build tool is unavailable, immediately stop and report the error.

---

# Dependency Rules

Running `go mod tidy` is allowed.

If it downloads Go modules, that is acceptable.

The agent must not:

* manually download dependencies
* install Go
* install Cargo
* install system packages
* install build tools
* install compilers

The agent must rely solely on the existing environment.

---

# Timing Rules

The only timing command allowed is:

`date`

After approximately every 200 lines read or written:

1. Execute `date`.
2. Re-read `RULES.md`.
3. Apply the newest rules immediately.
4. Print:

`Rules Read`

After every completed file:

1. Execute `date`.
2. Re-read `RULES.md`.
3. Print:

`Rules Read`

---

# Interruptions

If the user requests any action outside these rules, respond with exactly:

> I cannot do that under the current rules.

Do not explain further.

---

# Completion

The task is complete only when:

* every repository file has been read
* `NOTES.md` is complete
* Go migration is complete
* required builds/tests have completed

After completion the only allowed actions are:

* rereading `RULES.md`
* executing `date`
* following newly updated rules

No further work may be performed until new instructions are received.

---

# Agent Summary

1. Read `RULES.md`.
2. Read every repository file.
3. Never leave the repository.
4. Never inspect the host system.
5. Never search for executables.
6. Never modify PATH.
7. Never install software.
8. Never repair the environment.
9. Maintain `NOTES.md`.
10. Wait for permission.
11. Convert one Go file at a time.
12. Preserve behavior.
13. Build and test only with the permitted commands.
14. Re-read `RULES.md` at every timing checkpoint.
15. Stop immediately if a required tool is unavailable.
