package translator

import (
	"fmt"
	"unicode"

	"github.com/pilhuhn/otel-oql/pkg/sqlutil"
)

const maxAttributeKeyLen = 512
const maxPlainIdentifierLen = 128

// validatePlainIdentifier checks that s is a safe bare SQL identifier (letters, digits,
// underscore; first character must be letter or underscore).
func validatePlainIdentifier(s string) error {
	if s == "" {
		return fmt.Errorf("empty identifier")
	}
	if len(s) > maxPlainIdentifierLen {
		return fmt.Errorf("identifier too long")
	}
	for i, r := range s {
		if i == 0 {
			if !(unicode.IsLetter(r) || r == '_') {
				return fmt.Errorf("invalid identifier %q", s)
			}
			continue
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			return fmt.Errorf("invalid identifier %q", s)
		}
	}
	return nil
}

// validateAttributeKey allows arbitrary OTel-style keys (including Unicode) but rejects
// control characters and excessive length so values cannot break out of literals or markers.
func validateAttributeKey(key string) error {
	if key == "" {
		return fmt.Errorf("empty attribute key")
	}
	if len(key) > maxAttributeKeyLen {
		return fmt.Errorf("attribute key too long")
	}
	for _, r := range key {
		if unicode.IsControl(r) {
			return fmt.Errorf("invalid character in attribute key")
		}
	}
	return nil
}

// jsonPathLiteral returns a SQL string literal containing a JSON path for a top-level key
// (e.g. $.my.key). The key is embedded using standard SQL string escaping.
func jsonPathLiteral(attributeKey string) string {
	return sqlutil.StringLiteral("$." + attributeKey)
}
