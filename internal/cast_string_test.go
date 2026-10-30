package internal

import (
	"math"
	"strings"
	"testing"

	"github.com/goccy/go-googlesql"

	"github.com/goccy/googlesqlite/internal/value"
)

// These cases call CAST directly so they can check values the SQL layer
// cannot carry (NaN, -0) and NUMERIC range boundaries. Expected values follow the
// STRING conversion rules in
// docs/third_party/googlesql-docs/conversion_functions.md and match
// BigQuery's results.
func TestCAST_FromString(t *testing.T) {
	str := &Type{Kind: int(googlesql.TypeKindTypeString)}
	typeOf := func(k googlesql.TypeKind) *Type { return &Type{Kind: int(k)} }
	tests := []struct {
		in      string
		to      googlesql.TypeKind
		want    func(value.Value) bool
		wantErr string
	}{
		{in: "", to: googlesql.TypeKindTypeInt64, wantErr: "Bad int64 value: "},
		{in: "-0", to: googlesql.TypeKindTypeInt64, want: isInt(0)},
		{in: "-9223372036854775809", to: googlesql.TypeKindTypeInt64, wantErr: "Bad int64 value: -9223372036854775809"},
		{in: "", to: googlesql.TypeKindTypeBool, wantErr: "Bad bool value: "},
		{in: "fAlSe", to: googlesql.TypeKindTypeBool, want: func(v value.Value) bool { return v == value.BoolValue(false) }},
		// Only ASCII letters fold: U+017F LATIN SMALL LETTER LONG S is not 's'.
		{in: "falſe", to: googlesql.TypeKindTypeBool, wantErr: "Bad bool value: falſe"},
		{in: "", to: googlesql.TypeKindTypeDouble, wantErr: "Bad double value: "},
		{in: "NaN", to: googlesql.TypeKindTypeDouble, want: isNaN},
		{in: " -nan ", to: googlesql.TypeKindTypeDouble, want: isNaN},
		{in: "nan(1)", to: googlesql.TypeKindTypeDouble, want: isNaN},
		{in: "nan(", to: googlesql.TypeKindTypeDouble, wantErr: "Bad double value: nan("},
		{in: "0x.", to: googlesql.TypeKindTypeDouble, wantErr: "Bad double value: 0x."},
		{in: "0x1p", to: googlesql.TypeKindTypeDouble, wantErr: "Bad double value: 0x1p"},
		{in: "-0x1p", to: googlesql.TypeKindTypeDouble, wantErr: "Bad double value: -0x1p"},
		{in: "-0", to: googlesql.TypeKindTypeDouble, want: func(v value.Value) bool {
			f, ok := v.(value.FloatValue)
			return ok && f == 0 && math.Signbit(float64(f))
		}},
		{in: "", to: googlesql.TypeKindTypeNumeric, wantErr: "Invalid NUMERIC value: "},
		{in: "-0.0000000001", to: googlesql.TypeKindTypeNumeric, want: isNumeric("0", false)},
		{in: "0e99999999999", to: googlesql.TypeKindTypeNumeric, want: isNumeric("0", false)},
		{in: "1e-99999999999", to: googlesql.TypeKindTypeNumeric, want: isNumeric("0", false)},
		{in: "1e99999999999", to: googlesql.TypeKindTypeNumeric, wantErr: "Invalid NUMERIC value: 1e99999999999"},
		// Both sides of the early overflow cutoff in parseNumericString.
		{in: "1e70", to: googlesql.TypeKindTypeNumeric, wantErr: "Invalid NUMERIC value: 1e70"},
		{in: "1e71", to: googlesql.TypeKindTypeNumeric, wantErr: "Invalid NUMERIC value: 1e71"},
		{in: "-99999999999999999999999999999.9999999994", to: googlesql.TypeKindTypeNumeric, want: isNumeric("-99999999999999999999999999999.999999999", false)},
		{in: "5e-39", to: googlesql.TypeKindTypeBignumeric, want: isNumeric("0.00000000000000000000000000000000000001", true)},
		{in: "578960446186580977117854925043439539266.349923328202820197287920039565648199674", to: googlesql.TypeKindTypeBignumeric,
			want: isNumeric("578960446186580977117854925043439539266.34992332820282019728792003956564819967", true)},
		{in: "578960446186580977117854925043439539266.349923328202820197287920039565648199675", to: googlesql.TypeKindTypeBignumeric,
			wantErr: "Invalid BIGNUMERIC value: 578960446186580977117854925043439539266.349923328202820197287920039565648199675"},
		{in: "-578960446186580977117854925043439539266.34992332820282019728792003956564819969", to: googlesql.TypeKindTypeBignumeric,
			wantErr: "Invalid BIGNUMERIC value: -578960446186580977117854925043439539266.34992332820282019728792003956564819969"},
		{in: "", to: googlesql.TypeKindTypeString, want: func(v value.Value) bool { return v == value.StringValue("") }},
	}
	for _, tt := range tests {
		got, err := CAST(value.StringValue(tt.in), str, typeOf(tt.to), false)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("CAST(%q AS %v): got (%v, %v), want error containing %q", tt.in, tt.to, got, err, tt.wantErr)
			}
			if safe, err := CAST(value.StringValue(tt.in), str, typeOf(tt.to), true); safe != nil || err != nil {
				t.Errorf("SAFE_CAST(%q AS %v): got (%v, %v), want (nil, nil)", tt.in, tt.to, safe, err)
			}
			continue
		}
		if err != nil || !tt.want(got) {
			t.Errorf("CAST(%q AS %v): got (%#v, %v)", tt.in, tt.to, got, err)
		}
	}
}

// Non-STRING sources keep their existing conversions.
func TestCAST_NonStringSourceUnchanged(t *testing.T) {
	got, err := CAST(value.IntValue(1), &Type{Kind: int(googlesql.TypeKindTypeInt64)}, &Type{Kind: int(googlesql.TypeKindTypeBool)}, false)
	if err != nil || got != value.BoolValue(true) {
		t.Fatalf("CAST(1 AS BOOL): got (%v, %v), want true", got, err)
	}
}

func isInt(want int64) func(value.Value) bool {
	return func(v value.Value) bool { return v == value.IntValue(want) }
}

func isNaN(v value.Value) bool {
	f, ok := v.(value.FloatValue)
	return ok && math.IsNaN(float64(f))
}

func isNumeric(want string, isBigNumeric bool) func(value.Value) bool {
	return func(v value.Value) bool {
		n, ok := v.(*value.NumericValue)
		if !ok || n.IsBigNumeric != isBigNumeric {
			return false
		}
		s, err := n.ToString()
		return err == nil && s == want
	}
}
