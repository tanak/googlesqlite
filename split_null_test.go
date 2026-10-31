package googlesqlite_test

import (
	"database/sql"
	"testing"
)

// TestRegression_SplitNullReturnsNull asserts that SPLIT with a NULL argument
// returns NULL (not an empty array), so IFNULL(SPLIT(NULL, ","), ["Other"])
// falls back to the default array.
func TestRegression_SplitNullReturnsNull(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("googlesqlite", ":memory:?_test=splitnull")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	var isNull bool
	if err := db.QueryRow(`SELECT SPLIT(CAST(NULL AS STRING), ",") IS NULL`).Scan(&isNull); err != nil {
		t.Fatal(err)
	}
	if !isNull {
		t.Error("SPLIT(NULL, ',') IS NULL = false, want true")
	}

	var got string
	q := `SELECT g FROM (SELECT CAST(NULL AS STRING) AS genres), UNNEST(IFNULL(SPLIT(genres, ","), ["Other"])) AS g`
	if err := db.QueryRow(q).Scan(&got); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	if got != "Other" {
		t.Errorf("got %q, want Other", got)
	}
}
