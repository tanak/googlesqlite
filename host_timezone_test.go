package googlesqlite_test

import (
	"database/sql"
	"testing"
	"time"
)

// TestResultsIgnoreHostTimeZone checks that DATE/DATETIME/TIME views of
// a TIMESTAMP are computed in UTC, as BigQuery does, whatever TZ the
// process runs in. It mutates time.Local, so it must not run in parallel.
func TestResultsIgnoreHostTimeZone(t *testing.T) {
	saved := time.Local
	time.Local = time.FixedZone("JST", 9*60*60)
	defer func() { time.Local = saved }()

	db, err := sql.Open("googlesqlite", ":memory:?_test=host_timezone")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	tests := []struct{ name, sql, want string }{
		{"DATE of a timestamp column", `SELECT CAST(DATE(ts) AS STRING) FROM UNNEST([TIMESTAMP '2026-09-01 20:00:00+00']) AS ts`, "2026-09-01"},
		{"DATETIME of a timestamp column", `SELECT CAST(DATETIME(ts) AS STRING) FROM UNNEST([TIMESTAMP '2026-09-01 20:00:00+00']) AS ts`, "2026-09-01T20:00:00"},
		{"EXTRACT HOUR of a timestamp column", `SELECT CAST(EXTRACT(HOUR FROM ts) AS STRING) FROM UNNEST([TIMESTAMP '2026-09-01 20:00:00+00']) AS ts`, "20"},
		{"FORMAT_TIMESTAMP of a timestamp column", `SELECT FORMAT_TIMESTAMP('%F %T', ts) FROM UNNEST([TIMESTAMP '2026-09-01 20:00:00+00']) AS ts`, "2026-09-01 20:00:00"},
		{"TIMESTAMP_TRUNC to day", `SELECT FORMAT_TIMESTAMP('%F %T', TIMESTAMP_TRUNC(ts, DAY)) FROM UNNEST([TIMESTAMP '2026-09-01 20:00:00+00']) AS ts`, "2026-09-01 00:00:00"},
		{"TIMESTAMP_SECONDS to date", `SELECT CAST(DATE(TIMESTAMP_SECONDS(x)) AS STRING) FROM UNNEST([1788292800]) AS x`, "2026-09-01"},
		{"TIMESTAMP_MICROS to date", `SELECT CAST(DATE(TIMESTAMP_MICROS(x)) AS STRING) FROM UNNEST([1788292800000000]) AS x`, "2026-09-01"},
		{"DATE_FROM_UNIX_DATE", `SELECT CAST(DATE_FROM_UNIX_DATE(x) AS STRING) FROM UNNEST([20697]) AS x`, "2026-09-01"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			if err := db.QueryRow(tc.sql).Scan(&got); err != nil {
				t.Fatalf("query: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
