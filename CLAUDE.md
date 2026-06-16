# Meads (`md`) Task Tracking Context

## Overview

`md` is a git-native task tracker that stores all tasks in a single file — `TASKS.md` (Markdown) or `TASKS.csv` (CSV). No database, no config files — just your task file and git.

## Essential Commands

### Finding Work
- `md ready` - Show open tasks not blocked by dependencies (sorted by priority)
- `md list` - List all tasks
- `md list --json` - List all tasks as JSON
- `md get <id>` - Get a specific task
- `md get --json <id>` - Get a specific task as JSON

### Creating Tasks
- `md add "Fix the login bug"` - Add a simple task
- `md add "bug: Fix login P1. Session cookie expires"` - Rich input parsing
  - Type prefix: `bug:`, `task:`, `feature:` (optional)
  - Priority: `P0`-`P9` (0=critical, 4=backlog, default=P2)
  - Title: everything before first `.`
  - Description: everything after the `.`
- `md add --title="Fix login" --type=bug --priority=P1 --description="Details here"` - Flag-based
- `md add --title="Fix login" --description-file=/path/to/notes.md` - Description from file

### Updating Tasks
- `md update <id> --status=draft|open|inprogress|closed` - Update status
- `md update <id> --priority=P1` - Update priority
- `md update <id> --title="New title"` - Update title
- `md update <id> --description-file=/path/to/notes.md` - Update description from file
- `md set-status <id> <status>` - Shorthand for status changes
- `md del <id>` - Delete a task

### Dependencies
- `md add-dep <child> <parent>` - Make child depend on parent
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
- **Do NOT read or edit the task file (TASKS.md or TASKS.csv) directly** - always use `md` commands to read and modify tasks
- **ALWAYS include changes to the task file in the next git commit after task is closed**
- Task IDs are auto-assigned integers
- Git handles versioning and history
- The file uses optimistic locking for concurrent access
