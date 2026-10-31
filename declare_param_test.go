package googlesqlite_test

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

// TestDeclareDefaultQueryParameter checks that a DECLARE DEFAULT that
// reads a query parameter binds the caller's argument instead of
// evaluating the parameter to NULL.
func TestDeclareDefaultQueryParameter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sql  string
		args []any
		want []any
	}{
		{
			name: "int64",
			sql:  "DECLARE n INT64 DEFAULT @p; SELECT n + 1",
			args: []any{sql.Named("p", int64(41))},
			want: []any{int64(42)},
		},
		{
			name: "string",
			sql:  "DECLARE s STRING DEFAULT @p; SELECT s",
			args: []any{sql.Named("p", "x")},
			want: []any{"x"},
		},
		{
			name: "split of a parameter",
			sql:  "DECLARE a ARRAY<STRING> DEFAULT SPLIT(@p, '|'); SELECT a[0], a[1]",
			args: []any{sql.Named("p", "u|v")},
			want: []any{"u", "v"},
		},
		{
			name: "variable derived from a parameter variable",
			sql:  "DECLARE n INT64 DEFAULT @p; DECLARE m INT64 DEFAULT n * 2; SELECT m",
			args: []any{sql.Named("p", int64(5))},
			want: []any{int64(10)},
		},
		{
			name: "date derived from a parameter",
			sql:  "DECLARE d DATE DEFAULT PARSE_DATE('%Y-%m-%d', @p); SELECT CAST(DATE_ADD(d, INTERVAL 7 DAY) AS STRING)",
			args: []any{sql.Named("p", "2026-09-01")},
			want: []any{"2026-09-08"},
		},
		{
			name: "parameter name inside a string literal is not a parameter",
			sql:  "DECLARE s STRING DEFAULT '@p'; SELECT s",
			want: []any{"@p"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, err := sql.Open("googlesqlite", ":memory:?_test=declare_param_"+tc.name)
			if err != nil {
				t.Fatalf("sql.Open: %v", err)
			}
			defer db.Close()
			rows, err := db.QueryContext(context.Background(), tc.sql, tc.args...)
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			defer rows.Close()
			if !rows.Next() {
				t.Fatalf("no rows: %v", rows.Err())
			}
			got := make([]any, len(tc.want))
			ptrs := make([]any, len(got))
			for i := range got {
				ptrs[i] = &got[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatalf("scan: %v", err)
			}
			for i, g := range got {
				if b, ok := g.([]byte); ok {
					got[i] = string(b)
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestScriptCommentsDoNotBreakDeclare checks that comments containing
// `;` or quotes, including a trailing comment on a DECLARE line, do not
// split statements or leak into the DEFAULT expression.
func TestScriptCommentsDoNotBreakDeclare(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("googlesqlite", ":memory:?_test=script_comments")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	query := `DECLARE a INT64 DEFAULT 30; -- 30; it's a "comment"
DECLARE b INT64 DEFAULT a + 1 # trailing; hash comment
;
/* block; comment
   spanning 'lines' */
SELECT a, b, '-- not a comment; ok' AS s`
	var a, b int64
	var s string
	if err := db.QueryRowContext(context.Background(), query).Scan(&a, &b, &s); err != nil {
		t.Fatalf("query: %v", err)
	}
	if a != 30 || b != 31 || s != "-- not a comment; ok" {
		t.Fatalf("got a=%d b=%d s=%q", a, b, s)
	}
}

// TestScriptVariablesDoNotLeakAcrossPoolCheckouts checks that a variable
// declared by one request does not rewrite an unrelated request that
// reuses the pooled connection, while a pinned connection keeps its
// session variables.
func TestScriptVariablesDoNotLeakAcrossPoolCheckouts(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("googlesqlite", ":memory:?_test=script_var_leak")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, "DECLARE v INT64 DEFAULT 5"); err != nil {
		t.Fatalf("DECLARE: %v", err)
	}
	var got int64
	if err := db.QueryRowContext(ctx, "SELECT 1 AS v").Scan(&got); err != nil {
		t.Fatalf("query after DECLARE on another checkout: %v", err)
	}
	if got != 1 {
		t.Fatalf("got %d, want 1", got)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "DECLARE w INT64 DEFAULT 7"); err != nil {
		t.Fatalf("DECLARE on pinned conn: %v", err)
	}
	if err := conn.QueryRowContext(ctx, "SELECT w").Scan(&got); err != nil {
		t.Fatalf("SELECT w: %v", err)
	}
	if got != 7 {
		t.Fatalf("pinned session variable = %d, want 7", got)
	}
}
