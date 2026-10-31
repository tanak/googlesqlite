package bit

import "github.com/goccy/googlesqlite/internal/value"

// BIT_RIGHT_SHIFT shifts a right by b bits.
func BIT_RIGHT_SHIFT(a, b value.Value) (value.Value, error) {
	if ba, ok := a.(value.BytesValue); ok {
		n, err := b.ToInt64()
		if err != nil {
			return nil, err
		}
		return bytesShift(ba, n, false)
	}
	va, err := a.ToInt64()
	if err != nil {
		return nil, err
	}
	vb, err := b.ToInt64()
	if err != nil {
		return nil, err
	}
	return value.IntValue(va >> vb), nil
}
