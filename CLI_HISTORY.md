# OQL CLI History Support

The `oql-cli` now includes comprehensive command history support with both readline-like editing and c-shell style shortcuts.

## Features

### 1. Full Command-Line Editing (via liner library)

- **Arrow key navigation**: Use left/right arrows to move cursor within the current line
- **History navigation**: Use up/down arrows to browse through previous commands
- **Reverse-i-search**: Press `Ctrl+R` to search through command history
- **Backspace/Delete**: Edit commands naturally
- **Persistent history**: Commands are automatically saved to `~/.oql_history`

### 2. C-Shell Style History Commands

| Command | Description | Example |
|---------|-------------|---------|
| `!!` | Repeat last command | `oql> !!` |
| `!h` | Show command history with line numbers | `oql> !h` |
| `!<n>` | Run command number `n` from history | `oql> !5` |

## Installation

The feature uses the `github.com/peterh/liner` library (Apache 2.0 licensed):

```bash
go get github.com/peterh/liner
go build -o oql-cli ./cmd/oql-cli
```

## Usage Examples

### Example 1: Basic History Navigation

```bash
$ ./oql-cli --tenant-id=0

oql> signal=spans limit 10
[results displayed]

oql> [Press Up Arrow]
oql> signal=spans limit 10        # Previous command restored

oql> signal=traces where error=true
[results displayed]

oql> !!                            # Repeat last command
signal=traces where error=true
[results displayed]
```

### Example 2: History List and Replay

```bash
oql> signal=spans limit 10
[results]

oql> signal=metrics where value > 100
[results]

oql> signal=logs where severity_text = "ERROR"
[results]

oql> !h                            # Show history
Command History:
   1  signal=spans limit 10
   2  signal=metrics where value > 100
   3  signal=logs where severity_text = "ERROR"

oql> !2                            # Re-run command #2
signal=metrics where value > 100
[results]
```

### Example 3: Reverse-i-search

```bash
oql> signal=spans where service_name = "payment" limit 10
[results]

oql> signal=traces where duration > 5s
[results]

oql> [Press Ctrl+R]
(reverse-i-search)`pay': signal=spans where service_name = "payment" limit 10
[Press Enter to execute, or continue typing to refine search]
```

## Implementation Details

### Files Modified

- `cmd/oql-cli/main.go`: Added liner integration and history command handlers
- `cmd/oql-cli/README.md`: Updated documentation
- `cmd/oql-cli/history_test.go`: Comprehensive test coverage
- `CLAUDE.md`: Updated project documentation

### Key Functions

```go
// getHistoryFilePath returns ~/.oql_history path
func getHistoryFilePath() string

// handleHistoryCommand processes !!, !h, !<n> commands
// Returns command to execute, or "" for informational commands
func handleHistoryCommand(input string, history []string) string
```

### Testing

```bash
# Run history command tests
go test -v -run TestHandleHistoryCommand

# Test output shows:
# - !! repeats last command
# - !h shows history (with empty history handled)
# - !<n> runs specific command
# - Invalid numbers and out-of-range handled gracefully
```

## Benefits

1. **Better UX**: Natural command-line editing expected by users
2. **Productivity**: Quick access to previous queries without retyping
3. **Discoverability**: `!h` makes it easy to see what you've tried
4. **Scripting**: C-shell shortcuts work well in automation scenarios
5. **Minimal Dependencies**: Only one small, well-maintained library (liner)

## History File

Commands are persisted to `~/.oql_history` and automatically loaded on startup. The file uses a simple line-based format compatible with readline-style history.

## Error Handling

- Empty history: `!!` and `!<n>` provide helpful messages
- Invalid numbers: `!abc` shows "Invalid history number: abc"
- Out of range: `!99` shows valid range (e.g., "1-5")
- Ctrl+C: Aborts current line without exiting the shell
- Ctrl+D: Exits the shell gracefully

## Compatibility

- Works on macOS, Linux, and Windows
- Falls back gracefully if terminal doesn't support ANSI codes
- History file is plain text (no binary dependencies)
- Apache 2.0 license (compatible with project requirements)

## Future Enhancements

Potential improvements for future consideration:

1. **History search**: Add `!?pattern?` to search for commands containing pattern
2. **History size limit**: Configure max history entries (currently unlimited)
3. **History clearing**: Add `!c` to clear history
4. **History export**: Add `!e <file>` to export history to a file
5. **Tab completion**: Add completion for OQL keywords, table names, etc.
