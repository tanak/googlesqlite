package bit

import "github.com/goccy/googlesqlite/internal/value"

// BIT_OR returns the bitwise OR of two INT64 values.
func BIT_OR(a, b value.Value) (value.Value, error) {
	if ba, bb, ok := bytesArgs(a, b); ok {
		return bytesBinary(ba, bb, func(x, y byte) byte { return x | y })
	}
	va, err := a.ToInt64()
	if err != nil {
		return nil, err
	}
	vb, err := b.ToInt64()
	if err != nil {
		return nil, err
	}
	return value.IntValue(va | vb), nil
}
