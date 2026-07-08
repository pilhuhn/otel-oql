#!/bin/bash
# Demo script for oql-cli history features
# This shows the various history commands available

cat <<'EOF'
# OQL CLI History Demo
# ====================

# The oql-cli now supports full command-line editing with history!

# 1. Start the interactive CLI
./oql-cli --tenant-id=0

# 2. Run some queries
oql> signal=spans limit 10
oql> signal=metrics where value > 100
oql> signal=logs where severity_text = "ERROR"

# 3. Use Up/Down arrows to navigate history
oql> [Press Up Arrow]     # Shows previous command
oql> [Press Up Arrow]     # Shows command before that
oql> [Press Down Arrow]   # Move forward in history

# 4. C-shell style shortcuts

## Repeat last command
oql> !!
signal=logs where severity_text = "ERROR"

## Show history with line numbers
oql> !h
Command History:
   1  signal=spans limit 10
   2  signal=metrics where value > 100
   3  signal=logs where severity_text = "ERROR"

## Re-run a specific command by number
oql> !2
signal=metrics where value > 100

## Re-run first command
oql> !1
signal=spans limit 10

# 5. Reverse-i-search (Ctrl+R)
oql> [Press Ctrl+R]
(reverse-i-search)`spans': signal=spans limit 10
# Type to search, press Enter to execute, or Ctrl+C to cancel

# 6. Command-line editing
oql> signal=spans where service_name = "payment" limit 10
# Use left/right arrows to move cursor
# Use backspace/delete to edit
# Edit mid-line: [Left] [Left] [Backspace] [Type]

# 7. History persists across sessions
# Exit and restart oql-cli
oql> exit
$ ./oql-cli --tenant-id=0
# Press Up to see commands from previous session!

# 8. History file location
# Commands are saved to ~/.oql_history
$ cat ~/.oql_history
signal=spans limit 10
signal=metrics where value > 100
signal=logs where severity_text = "ERROR"

# Tips:
# - History is saved automatically on exit
# - Use Ctrl+C to cancel current line (not exit)
# - Use Ctrl+D to exit the CLI
# - Command history is shared across all sessions
# - Invalid history commands show helpful error messages

EOF
