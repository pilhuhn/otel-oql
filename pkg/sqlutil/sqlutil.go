package sqlutil

import "strings"

// StringLiteral returns a single-quoted SQL string literal with standard single-quote
// escaping (' → ”).
func StringLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// JSONObjectKeyPathLiteral returns a SQL string literal for a Pinot JSON path to a
// top-level key in JSON_EXTRACT_SCALAR (e.g. $.http.route for key "http.route").
// Single quotes in the key are escaped per SQL rules.
func JSONObjectKeyPathLiteral(key string) string {
	return StringLiteral("$." + key)
}
