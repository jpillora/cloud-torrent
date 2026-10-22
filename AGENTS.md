<!-- md-prime:start -->
# Meads (`md`) Task Tracking Context

## Overview

`md` is running in **git mode**: tasks live as git refs (`refs/meads/tasks/<id>`), not in a file. There is no `TASKS.md` or `TASKS.csv` in this repo — nothing to read or edit directly. Every command below is exactly the same as file mode; only the storage differs.

## Essential Commands

### Diagnostics
- `md --verbose <command> ...` / `md -V <command> ...` - Show task, webhook, and Git actions with elapsed timings on stderr (global flag; place it before the command). When remote sync runs, fetch and push are timed separately. Lowercase `-v` remains the version flag
- `md sync` - Synchronize `refs/meads/*` with `origin` now and exit non-zero on failure; use this explicit path when remote delivery must be confirmed

### Finding Work
- `md ready` - Show open tasks not blocked by dependencies (sorted by priority)
- `md list` - List all tasks
- `md list --json` - List all tasks as JSON
- `md list --limit=50 --offset=100` - Page filtered results (also supported by `md ready`)
- `md list --tag=api` - List tasks carrying a tag (comma-separated requires all of them, e.g. `--tag=api,backend`)
- `md ready --tag=api` - Same filter over ready work
- `md list --history` - List all tasks from git history (including deleted)
- `md get <id>` - Get a specific task (a soft-deleted task's ref is kept forever, so this still resolves it)
- `md get --json <id>` - Get a specific task as JSON

### Creating Tasks
- `md add "Fix the login bug"` - Add a simple task
- `md add "bug: Fix login P1. Session cookie expires"` - Rich input parsing
  - Type prefix: `bug:`, `task:`, `feature:`, `idea:` (optional)
  - Priority: `P0`-`P9` (0=critical, 4=backlog, default=P2)
  - Title: text before the first `. ` (period+space) or newline
  - Description: text after that split point
- `md add --title="Fix login" --type=bug --priority=P1 --description="Details here"` - Flag-based
- `md add --title="Fix login" --description-file=/path/to/notes.md` - Description from file
- For rich Markdown, read the description from a quoted HEREDOC so the shell passes backticks, dollar signs, and backslashes literally:
  ```bash
  md add --title="Fix login" --description-file=- <<'EOF'
  ## Context

  The session cookie expires while `document.hidden` is true.
  EOF
  ```
- `md add --title="Fix login" --tags=api,web-ui` - Set tags (comma-separated; each tag is lowercase letters, numbers and dashes)

### Updating Tasks
- `md update <id> --status=draft|open|inprogress|closed` - Update status
- `md update <id> --priority=P1` - Update priority
- `md update <id> --title="New title"` - Update title
- `md update <id> --description="Short details"` - Update a short description inline (JSON-style escapes such as `\n` still decode)
- For rich Markdown, prefer stdin via a quoted HEREDOC; `--description-file=-` means read stdin and does not require shell escaping:
  ```bash
  md update <id> --description-file=- <<'EOF'
  ## Notes

  - Preserves `code spans` literally
  - Supports real newlines without `\n` escapes
  EOF
  ```
- `md update <id> --description-file=/path/to/notes.md` - Update description from file
- `md update <id> --tags=api,web-ui` - Replace all tags (`--tags=` clears them)
- `md update <id> --add-tags=docs` / `md update <id> --rm-tags=api` - Add or remove tags, keeping the rest
- `md set-status <id> <status>` - Shorthand for status changes
- `md del <id>` - Delete a task (soft delete — see Rules)
- `md restore <id>` / `md restore --all` - Undo a soft delete, returning the task(s) to `md list`/`md ready`. `--dry-run` reports what it would restore
- `md del <id> --force` - Erase a task instead of tombstoning it — see Rules

### Dependencies
- `md add-dep <child> <parent>` - Make child depend on parent
- `md rm-dep <child> <parent>` - Remove child's dependency on parent
- `md add --depends-on=<id> "Task title"` - Add task with dependency
- Tasks blocked by unclosed dependencies are excluded from `md ready`

## Common Workflows

**Starting a session:**
```bash
md ready              # Find available work
md get <id>           # Review task details
md set-status <id> inprogress  # Claim it
```

**Creating dependent tasks:**
```bash
md add "feature: Build API endpoint"    # Returns ID, e.g. 5
md add "Write tests for API. Cover edge cases" --depends-on=5
```

**Completing work:**
```bash
md set-status <id> closed    # Mark task done
```

## Rules
- **There is no task file** - do NOT look for or try to edit `TASKS.md`/`TASKS.csv`; it does not exist in git mode. Always use `md` commands to read and modify tasks, never raw `git` plumbing on `refs/meads/*`.
- **Nothing to stage or commit yourself** - every `md add`/`update`/`set-status`/`add-dep`/`rm-dep`/`del` commits straight to that task's own ref the moment it runs. There is no "commit the tasks file" step.
- If `origin` is configured, a successful CLI write queues a best-effort detached `md sync`. Writes debounce for `pushInterval` (default 1m) and return without waiting for the network; a later write resets the timer, including while a sync is already running. Run `md sync` explicitly when you need a guaranteed attempt and trustworthy exit status.
- The background worker uses `MEADS_SYNC_PID` (default `~/.local/run/meads-sync.pid`), `MEADS_SYNC_DELAY` (overrides `pushInterval`), `MEADS_SYNC_TIMEOUT` (background default 10s), and `MEADS_SYNC_LOG` (default beside the PID file). Set `MEADS_SYNC_DISABLE=1` to disable background scheduling without disabling explicit `md sync`.
- `md del` never removes anything - it soft-deletes (the ref is kept forever), so a deleted id is never reused and `md get <id>` still resolves it. `md restore <id>` puts it back.
- `md del --force` is the exception, and the only irreversible command here: it deletes the ref outright, so `md get`, `md list --history` and `md restore` all lose the task together. It also gives up the never-reused guarantee — ids come from the highest ref that still exists, so erasing the highest id hands that number to the next `md add`, and anything already referring to it (a commit message, a branch name, a link in another task) silently retargets. Prefer `md del`.
- Concurrent writes are safe via compare-and-swap on each task's own ref.
- `md auto-save` and `md auto-delete` are file-mode git hooks; both no-op in git mode (there is no tasks file to stage or prune).
- `md beads-import` is not supported in git mode (it only imports into a tasks file).
- `md doctor` also detects tasks that have diverged after independent edits in two clones, and repairs duplicate ids left by two clones creating a task offline at the same id; a genuine divergence needs manual resolution.
- Task IDs are auto-assigned integers.
<!-- md-prime:end -->

## Development commands

Use `./dev.ts` (Take) for project commands; Bun 1.4.4+ is required.
Build commands and Docker, release, and type-generation configuration live in `.dev/`.

- `./dev.ts build` builds the frontend, then embeds it in `dist/cloud-torrent`.
- `./dev.ts gen-types` regenerates frontend types from Go structs.
- `./dev.ts check-types` regenerates types and fails if they differ from Git.
- `./dev.ts test` runs the Go test suite.
- `./dev.ts ci` runs type checks, the build, and tests in sequence.
- `./dev.ts release` publishes a tagged prerelease with GoReleaser.

Run `./dev.ts --help` for the full command list. The entrypoint sets the repo
root as the working directory, so it can also be invoked by absolute path.
