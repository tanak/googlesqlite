package googlesqlite_test

import (
	"database/sql"
	"math"
	"testing"
)

// TestRegression_AggregatesIntegralFloatFirst covers aggregates other than SUM
// that share IntValue.Add: an integral FLOAT64 first must not truncate later
// fractional values.
func TestRegression_AggregatesIntegralFloatFirst(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("googlesqlite", ":memory:?_test=aggmixedfloat")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	const rows = "UNNEST([STRUCT<k INT64, x FLOAT64>(1, 1500.0), (2, 2.4), (3, 3.1)]) AS s"
	for _, tc := range []struct {
		name, q string
		want    float64
	}{
		{"avg", "SELECT AVG(x) FROM " + rows, 1505.5 / 3},
		{"avg float first", "SELECT AVG(x) FROM UNNEST([2.4, 1500.0, 3.1]) AS x", 1505.5 / 3},
		{"avg distinct", "SELECT AVG(DISTINCT x) FROM UNNEST([1500.0, 2.4, 2.4, 3.1]) AS x", 1505.5 / 3},
		{"sum distinct", "SELECT SUM(DISTINCT x) FROM UNNEST([1500.0, 2.4, 2.4, 3.1]) AS x", 1505.5},
		{"window sum", "SELECT SUM(x) OVER (ORDER BY k ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) FROM " + rows + " ORDER BY k DESC LIMIT 1", 1505.5},
		{"window sum whole", "SELECT SUM(x) OVER () FROM " + rows + " LIMIT 1", 1505.5},
		{"window sum frame", "SELECT v FROM (SELECT k, SUM(x) OVER (ORDER BY k ROWS BETWEEN 1 PRECEDING AND CURRENT ROW) AS v FROM " + rows + ") WHERE k=2", 1502.4},
		{"window sum distinct", "SELECT SUM(DISTINCT x) OVER () FROM UNNEST([1500.0, 2.4, 2.4, 3.1]) AS x LIMIT 1", 1505.5},
		{"window avg", "SELECT AVG(x) OVER () FROM " + rows + " LIMIT 1", 1505.5 / 3},
		{"window avg frame", "SELECT v FROM (SELECT k, AVG(x) OVER (ORDER BY k ROWS BETWEEN 1 PRECEDING AND CURRENT ROW) AS v FROM " + rows + ") WHERE k=2", 1502.4 / 2},
		{"min", "SELECT MIN(x) FROM " + rows, 2.4},
		{"max", "SELECT MAX(x) FROM UNNEST([2.4, 1500.0, 3.1]) AS x", 1500},
	} {
		var got float64
		if err := db.QueryRow(tc.q).Scan(&got); err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", tc.name, got, tc.want)
		}
	}
}
