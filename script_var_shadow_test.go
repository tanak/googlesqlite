package googlesqlite_test

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

// TestScriptVariableShadowedByAliasOrCTE checks that a column alias or
// CTE that shares its name with a script variable stays an alias/CTE,
// while UNNEST(<name>) with no column in scope still reads the variable.
func TestScriptVariableShadowedByAliasOrCTE(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sql  string
		want [][]any
	}{
		{
			name: "alias with the variable's name",
			sql:  "DECLARE label STRING DEFAULT 'v'; SELECT label FROM (SELECT 'c' AS label)",
			want: [][]any{{"c"}},
		},
		{
			name: "cte with the variable's name",
			sql: "DECLARE xs ARRAY<STRING> DEFAULT ['a', 'b'];" +
				"WITH xs AS (SELECT 'q' AS v) SELECT v FROM xs",
			want: [][]any{{"q"}},
		},
		{
			name: "unnest of the variable next to an alias of the same name",
			sql: "DECLARE xs ARRAY<STRING> DEFAULT ['a', 'b'];" +
				"WITH t AS (SELECT 'q' AS xs) SELECT xs, x FROM t, UNNEST(xs) AS x ORDER BY x",
			want: [][]any{{"q", "a"}, {"q", "b"}},
		},
		{
			name: "unshadowed variable is still substituted",
			sql:  "DECLARE n INT64 DEFAULT 3; SELECT n AS m",
			want: [][]any{{int64(3)}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, err := sql.Open("googlesqlite", ":memory:?_test=shadow_"+tc.name)
			if err != nil {
				t.Fatalf("sql.Open: %v", err)
			}
			defer db.Close()
			rows, err := db.QueryContext(context.Background(), tc.sql)
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			defer rows.Close()
			var got [][]any
			for rows.Next() {
				cols, _ := rows.Columns()
				row := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range row {
					ptrs[i] = &row[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					t.Fatalf("scan: %v", err)
				}
				got = append(got, row)
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("rows: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
