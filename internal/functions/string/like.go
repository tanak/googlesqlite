package string

import (
	"regexp"
	"strings"

	"github.com/goccy/googlesqlite/internal/functions/helper"
	"github.com/goccy/googlesqlite/internal/value"
)

func LIKE(a, b value.Value) (value.Value, error) {
	va, err := a.ToString()
	if err != nil {
		return nil, err
	}
	vb, err := b.ToString()
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(likePatternToRegexp(vb))
	if err != nil {
		return nil, err
	}
	return value.BoolValue(re.MatchString(va)), nil
}

// likePatternToRegexp translates a GoogleSQL LIKE pattern: `%` matches any
// sequence, `_` matches exactly one character, and a backslash escapes
// `%`, `_` and itself. Every other character is matched literally.
func likePatternToRegexp(pattern string) string {
	var b strings.Builder
	b.WriteString("(?s)^")
	rs := []rune(pattern)
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; r {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteString(".")
		case '\\':
			if i+1 < len(rs) && (rs[i+1] == '%' || rs[i+1] == '_' || rs[i+1] == '\\') {
				i++
				r = rs[i]
			}
			b.WriteString(regexp.QuoteMeta(string(r)))
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return b.String()
}

// BindLike: per GoogleSQL three-valued logic, LIKE with a NULL operand
// returns NULL, not FALSE — Scalar2 propagates that.
var BindLike = helper.Scalar2(LIKE)
