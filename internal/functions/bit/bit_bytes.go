package bit

import (
	"errors"
	"fmt"

	"github.com/goccy/googlesqlite/internal/value"
)

// bytesBinary applies op byte by byte. The inputs may be shared with the
// caller, so the result is always a fresh slice.
func bytesBinary(a, b value.BytesValue, op func(x, y byte) byte) (value.Value, error) {
	if len(a) != len(b) {
		return nil, fmt.Errorf(
			"Bitwise binary operator for BYTES requires equal length of the inputs. Got %d bytes on the left hand side and %d bytes on the right hand side.",
			len(a), len(b),
		)
	}
	out := make(value.BytesValue, len(a))
	for i := range a {
		out[i] = op(a[i], b[i])
	}
	return out, nil
}

func bytesNot(a value.BytesValue) value.Value {
	out := make(value.BytesValue, len(a))
	for i, c := range a {
		out[i] = ^c
	}
	return out
}

// bytesShift treats a as a big-endian bit string and keeps its length.
// Bits shifted out are dropped and the vacated bits are zero.
func bytesShift(a value.BytesValue, n int64, left bool) (value.Value, error) {
	if n < 0 {
		return nil, errors.New("Bitwise shift by negative offset.")
	}
	out := make(value.BytesValue, len(a))
	if n >= int64(len(a))*8 {
		return out, nil
	}
	byteShift, bitShift := int(n/8), uint(n%8)
	for i := range out {
		src := i + byteShift
		if left {
			var v byte
			if src < len(a) {
				v = a[src] << bitShift
				if src+1 < len(a) && bitShift > 0 {
					v |= a[src+1] >> (8 - bitShift)
				}
			}
			out[i] = v
			continue
		}
		src = i - byteShift
		var v byte
		if src >= 0 {
			v = a[src] >> bitShift
			if src-1 >= 0 && bitShift > 0 {
				v |= a[src-1] << (8 - bitShift)
			}
		}
		out[i] = v
	}
	return out, nil
}

func bytesArgs(a, b value.Value) (value.BytesValue, value.BytesValue, bool) {
	ba, ok := a.(value.BytesValue)
	if !ok {
		return nil, nil, false
	}
	bb, ok := b.(value.BytesValue)
	return ba, bb, ok
}
