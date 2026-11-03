package app_test

import (
	"strings"
	"testing"
)

// The admin list's sorts and the title-purge range scan must use an index
// (they used to scan + temp-sort every row of the kind).
func TestQueryPlans_UseIndexes(t *testing.T) {
	db := openLockTestDB(t)
	for name, q := range map[string]string{
		"list last_accessed": "SELECT cache_key FROM image_meta WHERE image_type = 'p' ORDER BY last_accessed DESC, cache_key DESC LIMIT 50 OFFSET 0",
		"list updated_at":    "SELECT cache_key FROM image_meta WHERE image_type = 'p' ORDER BY updated_at ASC, cache_key ASC LIMIT 50",
		"list release_date":  "SELECT cache_key FROM image_meta WHERE image_type = 'p' ORDER BY release_date DESC, cache_key DESC LIMIT 50",
		"list created_at":    "SELECT cache_key FROM image_meta WHERE image_type = 'p' ORDER BY created_at DESC, cache_key DESC LIMIT 50",
		"purge range":        "SELECT cache_key FROM image_meta WHERE +image_type = 'p' AND cache_key >= 'imdb/tt1' AND cache_key < 'imdb/tt2'",
	} {
		rows, err := db.Query("EXPLAIN QUERY PLAN " + q)
		if err != nil {
			t.Fatal(err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, notused int
			var detail string
			rows.Scan(&id, &parent, &notused, &detail)
			plan = append(plan, detail)
		}
		rows.Close()
		joined := strings.Join(plan, " | ")
		bad := strings.Contains(joined, "TEMP B-TREE") || !strings.Contains(joined, "INDEX")
		if name == "purge range" && !strings.Contains(joined, "cache_key>? AND cache_key<?") {
			bad = true // must be a key-range search, not a scan of the kind
		}
		if bad {
			t.Errorf("%s: plan %q", name, joined)
		} else {
			t.Logf("%s: %s", name, joined)
		}
	}
}
