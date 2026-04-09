package sqlutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringLiteral(t *testing.T) {
	assert.Equal(t, "''", StringLiteral(""))
	assert.Equal(t, "'hello'", StringLiteral("hello"))
	assert.Equal(t, "'a''b'", StringLiteral("a'b"))
	assert.Equal(t, "''''", StringLiteral("'"))
}

func TestJSONObjectKeyPathLiteral(t *testing.T) {
	assert.Equal(t, "'$.http.route'", JSONObjectKeyPathLiteral("http.route"))
	assert.Equal(t, "'$.a''b'", JSONObjectKeyPathLiteral("a'b"))
}
