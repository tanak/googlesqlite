package googlesqlite_test

import (
	"database/sql"
	"math"
	"testing"
)

// TestRegression_SumIntegralFloatFirst asserts that SUM over FLOAT64 values
// keeps the fractional part when the first value is integral.
//
// Origin: an integral FLOAT64 (1500.0) reaches the aggregator as an IntValue,
// SUM seeds with it, and IntValue.Add then truncated every later FLOAT64 via
// ToInt64, so SUM([1500.0, 2.4]) returned 1502 while the reverse order
// returned 1502.4.
func TestRegression_SumIntegralFloatFirst(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("googlesqlite", ":memory:?_test=summixedfloat")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	for _, q := range []string{
		"SELECT SUM(x) FROM UNNEST([1500.0, 2.4]) AS x",
		"SELECT SUM(x) FROM UNNEST([2.4, 1500.0]) AS x",
		"SELECT SUM(s.x) FROM UNNEST([STRUCT<x FLOAT64>(1500), STRUCT<x FLOAT64>(2.4)]) AS s",
		"SELECT SUM(COALESCE((SELECT s.x FROM UNNEST([STRUCT<x FLOAT64>(1500)]) AS s WHERE k=1), 2.4)) FROM UNNEST([1,2]) AS k",
	} {
		var got float64
		if err := db.QueryRow(q).Scan(&got); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if math.Abs(got-1502.4) > 1e-9 {
			t.Errorf("%s = %v, want 1502.4", q, got)
		}
	}
}
