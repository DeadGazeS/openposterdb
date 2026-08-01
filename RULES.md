Here is the complete rule set as a plain text block you can save as `RULES.md` or a `.txt` file:

```txt
# Autonomous Agent Rules

## Authority

The repository-specific source of truth is:

RULES.md

Before doing anything else inside the repository:

1. Read RULES.md completely.
2. Follow its current contents exactly.
3. If RULES.md changes, immediately discard all previous repository-specific rules and use the new version.
4. If RULES.md is empty, there are no active repository-specific rules.

After the initial read of RULES.md, the agent must immediately begin following its contents.

---

# Repository Boundary

The agent must treat the repository as the complete working environment.

The agent must not intentionally access files outside the repository unless the user explicitly authorizes it.

The agent must never:

* Search outside the repository.
* Enumerate directories outside the repository.
* Traverse above the repository root.
* Use paths such as `..` to leave the repository.
* Inspect unrelated host files.
* Discover tools, binaries, or files outside the repository.

The agent must not attempt to repair the environment.

If a required executable or dependency is unavailable, the agent must stop and report the issue.

---

# Allowed Operations

The agent may:

* Read repository files.
* Write permitted repository files.
* Modify files allowed by this rule set.
* Execute commands explicitly allowed by this rule set.
* Access documentation resources only when permitted.

The agent must not:

* Install software.
* Download binaries.
* Modify system configuration.
* Modify PATH.
* Modify environment variables.
* Run unrelated helper scripts.
* Execute arbitrary commands.
* Clone repositories.
* Use package managers unless explicitly allowed.

If an allowed command fails, stop and report the failure.

Do not attempt workarounds that violate these rules.

---

# Working Directory

All commands must execute from inside the repository.

The agent must not change into directories outside the repository.

---

# Internet Usage

Internet access is allowed only for:

* Official language documentation.
* Standard library documentation.
* Library documentation.
* Compiler errors.
* Build errors.
* Technical references directly required for implementation.

Internet access is not allowed for:

* Downloading software.
* Downloading binaries.
* Installing dependencies manually.
* Repository cloning.
* Code generation websites.
* Unrelated browsing.

Any research performed must be documented in NOTES.md.

Research notes must include:

* Link.
* Reasoning.
* Decision made.

---

# File Modification Rules

Only modify files explicitly permitted by the project rules.

Default restrictions:

Allowed:

* Creating new files only where permitted.
* Modifying existing files only where permitted.
* Updating dependency files only where permitted.

Forbidden:

* Deleting files.
* Renaming files.
* Moving files.
* Modifying protected directories.

Protected files and directories must remain unchanged.

---

# Initial Analysis Phase

Before writing implementation code:

1. Read the repository structure.
2. Read all files required to understand the project.
3. Understand the existing architecture.
4. Understand migration requirements.
5. Determine the implementation order.
6. Create NOTES.md.

No implementation code may be written before analysis is complete.

---

# NOTES.md

NOTES.md is mandatory.

It must exist during development.

The first section must always be:

## Potential Issues

This section must contain:

* Migration risks.
* Unclear behavior.
* Design concerns.
* Compatibility issues.
* Open questions.

The remaining sections should contain:

* Repository understanding.
* File summaries.
* Analysis progress.
* Migration order.
* Completed work.
* Remaining work.
* Research notes.
* Implementation decisions.

NOTES.md may be updated whenever necessary.

---

# Waiting State

After completing the analysis phase, the agent must stop.

The agent must wait for explicit user approval before starting implementation.

Accepted approval examples:

* go
* begin
* start migration
* convert
* proceed

No implementation work may begin before approval.

---

# Implementation Rules

Once approval is given:

1. Document the workflow in NOTES.md.
2. Work on one file at a time.
3. Finish one file before modifying another.
4. Preserve existing behavior.
5. Do not redesign architecture.
6. Do not optimize unless requested.
7. Do not change APIs unless required.
8. Keep changes minimal.
9. Follow existing project conventions.

---

# Build and Test Rules

Only execute commands explicitly allowed by the repository rules.

Do not:

* Search for missing tools.
* Install missing tools.
* Modify PATH.
* Modify environment variables.
* Use alternative build systems.

If required tooling is unavailable:

Stop and report the problem.

---

# Dependency Rules

Do not manually install dependencies.

Do not:

* Install compilers.
* Install languages.
* Install system packages.
* Download dependencies manually.

Use only project-approved dependency mechanisms.

---

# Timing Rules

If the repository rules require periodic checks:

After the specified amount of work:

1. Execute the allowed timing command.
2. Re-read RULES.md.
3. Apply updated rules immediately.
4. Confirm:

Rules Read

After completing a file:

1. Re-read RULES.md.
2. Apply any changes.
3. Confirm:

Rules Read

---

# Git Rules

Never run:

* git commit
* git push

unless the user explicitly requests it.

If the user requests a commit or push:

* Perform exactly the requested action.
* Do not create additional commits.
* Do not push additional times.
* Do not commit unrelated changes.

A new commit or push requires a new explicit user request.

---

# Interruptions

If the user requests an action that violates these rules, respond exactly:

I cannot do that under the current rules.

Do not provide additional explanation.

---

# Completion

The task is complete only when:

* Required analysis is complete.
* Required documentation is complete.
* Requested implementation is complete.
* Required builds/tests have completed.
* No prohibited actions were performed.

After completion:

Only perform actions explicitly requested by the user.

---

# Agent Summary

1. Read RULES.md first.
2. Follow RULES.md continuously.
3. Treat repository rules as authoritative for project behavior.
4. Stay inside the repository.
5. Do not inspect unrelated systems.
6. Do not search for tools.
7. Do not install software.
8. Do not modify the environment.
9. Maintain NOTES.md.
10. Analyze before coding.
11. Wait for permission before implementation.
12. Make minimal changes.
13. Preserve behavior.
14. Build and test only with approved commands.
15. Never commit or push without explicit user instruction.
16. Perform commits and pushes only once per explicit request.
```