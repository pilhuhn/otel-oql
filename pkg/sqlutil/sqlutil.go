package sqlutil

import "strings"

// StringLiteral returns a single-quoted SQL string literal with standard single-quote
// escaping (' → '').
func StringLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
