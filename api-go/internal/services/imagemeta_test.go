package services

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

const imageMetaTestSchema = `
CREATE TABLE IF NOT EXISTS image_meta (
	cache_key TEXT PRIMARY KEY,
	release_date TEXT,
	image_type TEXT NOT NULL DEFAULT 'poster',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	last_accessed INTEGER NOT NULL DEFAULT 0
);
`

func newImageMetaTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(imageMetaTestSchema); err != nil {
		t.Fatal(err)
	}
	return db
}

// seedImageMetaRow inserts a single image_meta row with the given fields.
// Caller is responsible for unique cache_keys across the test.
func seedImageMetaRow(t *testing.T, db *sql.DB, cacheKey, imageType string, releaseDate *string, createdAt, updatedAt, lastAccessed int64) {
	t.Helper()
	var rd interface{}
	if releaseDate != nil {
		rd = *releaseDate
	}
	if _, err := db.Exec(
		`INSERT INTO image_meta (cache_key, image_type, release_date, created_at, updated_at, last_accessed) VALUES (?, ?, ?, ?, ?, ?)`,
		cacheKey, imageType, rd, createdAt, updatedAt, lastAccessed,
	); err != nil {
		t.Fatalf("seed %q: %v", cacheKey, err)
	}
}

func readLastAccessed(t *testing.T, db *sql.DB, cacheKey string) int64 {
	t.Helper()
	var v int64
	if err := db.QueryRow("SELECT last_accessed FROM image_meta WHERE cache_key = ?", cacheKey).Scan(&v); err != nil {
		t.Fatalf("read last_accessed %q: %v", cacheKey, err)
	}
	return v
}

func TestTouchImageAccess_Throttle(t *testing.T) {
	db := newImageMetaTestDB(t)
	defer db.Close()

	now := nowUnix()
	seedImageMetaRow(t, db, "k1", "poster", nil, now, now, now)

	if err := TouchImageAccess(db, "k1"); err != nil {
		t.Fatalf("TouchImageAccess: %v", err)
	}
	if got := readLastAccessed(t, db, "k1"); got != now {
		t.Errorf("last_accessed changed within throttle window: got %d, want %d", got, now)
	}
}

func TestTouchImageAccess_FiresAfter60s(t *testing.T) {
	db := newImageMetaTestDB(t)
	defer db.Close()

	now := nowUnix()
	stale := now - 120
	seedImageMetaRow(t, db, "k1", "poster", nil, now, now, stale)

	before := nowUnix()
	if err := TouchImageAccess(db, "k1"); err != nil {
		t.Fatalf("TouchImageAccess: %v", err)
	}
	after := nowUnix()

	got := readLastAccessed(t, db, "k1")
	if got < before || got > after {
		t.Errorf("last_accessed not refreshed into [before, after] window: got %d, want in [%d, %d]", got, before, after)
	}
}

func TestTouchImageAccess_NoRow(t *testing.T) {
	db := newImageMetaTestDB(t)
	defer db.Close()

	if err := TouchImageAccess(db, "nonexistent"); err != nil {
		t.Errorf("TouchImageAccess on missing key should be a no-op, got: %v", err)
	}
}

func TestListImageMetaByKind_Sort(t *testing.T) {
	type columnCase struct {
		name      string
		sortBy    string
		// build inserts three rows with ascending keys ["a", "b", "c"] whose
		// sort-column values are ascending low→high. The test asserts that
		// ListImageMetaByKind returns them in either the same or the reverse
		// order, depending on the requested direction.
		ascLow, ascMid, ascHigh any
		// set when the sort column is release_date (TEXT).
		ascLowStr, ascMidStr, ascHighStr string
	}

	cases := []columnCase{
		{name: "release_date", sortBy: "release_date",
			ascLowStr: "2020-01-01", ascMidStr: "2021-01-01", ascHighStr: "2022-01-01"},
		{name: "created_at", sortBy: "created_at",
			ascLow: int64(100), ascMid: int64(200), ascHigh: int64(300)},
		{name: "updated_at", sortBy: "updated_at",
			ascLow: int64(100), ascMid: int64(200), ascHigh: int64(300)},
		{name: "last_accessed", sortBy: "last_accessed",
			ascLow: int64(0), ascMid: int64(120), ascHigh: int64(240)},
	}

	for _, cc := range cases {
		for _, dir := range []string{"ASC", "DESC"} {
			t.Run(cc.name+"_"+dir, func(t *testing.T) {
				db := newImageMetaTestDB(t)
				defer db.Close()

				// Constants for the non-sorted columns.
				const otherRelease = "2010-01-01"
				const otherInt int64 = 50

				seedOne := func(key string, sortVal any) {
					var rd *string
					var createdAt, updatedAt, lastAccessed int64 = otherInt, otherInt, otherInt
					if rdStr := otherRelease; rdStr != "" {
						rd = &rdStr
					}
					switch cc.sortBy {
					case "release_date":
						rd = &cc.ascLowStr
						if key == "b" {
							rd = &cc.ascMidStr
						} else if key == "c" {
							rd = &cc.ascHighStr
						}
					case "created_at":
						createdAt = cc.ascLow.(int64)
						if key == "b" {
							createdAt = cc.ascMid.(int64)
						} else if key == "c" {
							createdAt = cc.ascHigh.(int64)
						}
					case "updated_at":
						updatedAt = cc.ascLow.(int64)
						if key == "b" {
							updatedAt = cc.ascMid.(int64)
						} else if key == "c" {
							updatedAt = cc.ascHigh.(int64)
						}
					case "last_accessed":
						lastAccessed = cc.ascLow.(int64)
						if key == "b" {
							lastAccessed = cc.ascMid.(int64)
						} else if key == "c" {
							lastAccessed = cc.ascHigh.(int64)
						}
					}
					seedImageMetaRow(t, db, key, "poster", rd, createdAt, updatedAt, lastAccessed)
				}

				seedOne("a", nil)
				seedOne("b", nil)
				seedOne("c", nil)

				items, _, err := ListImageMetaByKind(db, "poster", cc.sortBy, dir, 1, 10)
				if err != nil {
					t.Fatalf("ListImageMetaByKind: %v", err)
				}
				if len(items) != 3 {
					t.Fatalf("got %d items, want 3", len(items))
				}

				want := []string{"a", "b", "c"}
				if dir == "DESC" {
					want = []string{"c", "b", "a"}
				}
				for i, key := range want {
					if items[i].CacheKey != key {
						keys := []string{items[0].CacheKey, items[1].CacheKey, items[2].CacheKey}
						t.Errorf("%s %s: order %v, want %v", cc.sortBy, dir, keys, want)
						break
					}
				}
			})
		}
	}

	t.Run("invalid_sort_by_defaults_to_created_at_DESC", func(t *testing.T) {
		db := newImageMetaTestDB(t)
		defer db.Close()

		// created_at: a=oldest, b=mid, c=newest. Bad sort_by should fall back
		// to created_at DESC → [c, b, a].
		release := "2020-01-01"
		seedImageMetaRow(t, db, "a", "poster", &release, 100, 100, 0)
		seedImageMetaRow(t, db, "b", "poster", &release, 200, 200, 0)
		seedImageMetaRow(t, db, "c", "poster", &release, 300, 300, 0)

		items, _, err := ListImageMetaByKind(db, "poster", "not_a_column", "DESC", 1, 10)
		if err != nil {
			t.Fatalf("ListImageMetaByKind: %v", err)
		}
		want := []string{"c", "b", "a"}
		for i, key := range want {
			if items[i].CacheKey != key {
				t.Errorf("order: got %v, want %v", keys(items), want)
				break
			}
		}
	})

	t.Run("invalid_sort_dir_defaults_to_DESC", func(t *testing.T) {
		db := newImageMetaTestDB(t)
		defer db.Close()

		release := "2020-01-01"
		seedImageMetaRow(t, db, "a", "poster", &release, 100, 100, 0)
		seedImageMetaRow(t, db, "b", "poster", &release, 200, 200, 0)
		seedImageMetaRow(t, db, "c", "poster", &release, 300, 300, 0)

		items, _, err := ListImageMetaByKind(db, "poster", "created_at", "garbage", 1, 10)
		if err != nil {
			t.Fatalf("ListImageMetaByKind: %v", err)
		}
		want := []string{"c", "b", "a"}
		for i, key := range want {
			if items[i].CacheKey != key {
				t.Errorf("order: got %v, want %v", keys(items), want)
				break
			}
		}
	})

	// Tied sort-column values fall back to `cache_key <dir>` so that flipping
	// the direction visibly reverses the list even when every row ties on the
	// sort column (e.g. same-second created_at, or last_accessed = 0).
	t.Run("ties_fall_back_to_cache_key_in_sort_direction", func(t *testing.T) {
		db := newImageMetaTestDB(t)
		defer db.Close()

		release := "2020-01-01"
		// Identical created_at AND updated_at AND last_accessed on all rows.
		seedImageMetaRow(t, db, "a", "poster", &release, 100, 100, 0)
		seedImageMetaRow(t, db, "b", "poster", &release, 100, 100, 0)
		seedImageMetaRow(t, db, "c", "poster", &release, 100, 100, 0)

		for _, sortBy := range []string{"created_at", "updated_at", "last_accessed"} {
			items, _, err := ListImageMetaByKind(db, "poster", sortBy, "ASC", 1, 10)
			if err != nil {
				t.Fatalf("ListImageMetaByKind: %v", err)
			}
			if got := keys(items); got[0] != "a" || got[1] != "b" || got[2] != "c" {
				t.Errorf("%s ASC ties: got %v, want [a b c]", sortBy, got)
			}

			items, _, err = ListImageMetaByKind(db, "poster", sortBy, "DESC", 1, 10)
			if err != nil {
				t.Fatalf("ListImageMetaByKind: %v", err)
			}
			if got := keys(items); got[0] != "c" || got[1] != "b" || got[2] != "a" {
				t.Errorf("%s DESC ties: got %v, want [c b a]", sortBy, got)
			}
		}
	})

	// Regression (2026-08-11): the web UI sends lowercase "asc"/"desc", but
	// the allowlist only held uppercase — every UI direction flip silently
	// fell back to DESC and sorting appeared broken in the live app while
	// the uppercase-only unit tests stayed green.
	t.Run("lowercase_direction_is_normalized", func(t *testing.T) {
		db := newImageMetaTestDB(t)
		defer db.Close()

		release := "2020-01-01"
		seedImageMetaRow(t, db, "a", "poster", &release, 100, 100, 0)
		seedImageMetaRow(t, db, "b", "poster", &release, 200, 200, 0)
		seedImageMetaRow(t, db, "c", "poster", &release, 300, 300, 0)

		items, _, err := ListImageMetaByKind(db, "poster", "created_at", "asc", 1, 10)
		if err != nil {
			t.Fatalf("ListImageMetaByKind: %v", err)
		}
		if got := keys(items); got[0] != "a" || got[1] != "b" || got[2] != "c" {
			t.Errorf("created_at asc (lowercase): got %v, want [a b c]", got)
		}

		items, _, err = ListImageMetaByKind(db, "poster", "created_at", "desc", 1, 10)
		if err != nil {
			t.Fatalf("ListImageMetaByKind: %v", err)
		}
		if got := keys(items); got[0] != "c" || got[1] != "b" || got[2] != "a" {
			t.Errorf("created_at desc (lowercase): got %v, want [c b a]", got)
		}
	})

	// Lock in SQLite NULL ordering for release_date so the frontend's
	// within-group comparator can mirror it: NULLs sort FIRST in ASC and LAST
	// in DESC.
	t.Run("release_date_nulls_first_in_asc_last_in_desc", func(t *testing.T) {
		db := newImageMetaTestDB(t)
		defer db.Close()

		early := "2020-01-01"
		late := "2022-01-01"
		seedImageMetaRow(t, db, "null-row", "poster", nil, 100, 100, 0)
		seedImageMetaRow(t, db, "early-row", "poster", &early, 100, 100, 0)
		seedImageMetaRow(t, db, "late-row", "poster", &late, 100, 100, 0)

		items, _, err := ListImageMetaByKind(db, "poster", "release_date", "ASC", 1, 10)
		if err != nil {
			t.Fatalf("ListImageMetaByKind: %v", err)
		}
		if got := keys(items); got[0] != "null-row" || got[1] != "early-row" || got[2] != "late-row" {
			t.Errorf("release_date ASC: got %v, want [null-row early-row late-row]", got)
		}

		items, _, err = ListImageMetaByKind(db, "poster", "release_date", "DESC", 1, 10)
		if err != nil {
			t.Fatalf("ListImageMetaByKind: %v", err)
		}
		if got := keys(items); got[0] != "late-row" || got[1] != "early-row" || got[2] != "null-row" {
			t.Errorf("release_date DESC: got %v, want [late-row early-row null-row]", got)
		}
	})
}

func keys(items []ImageMetaItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.CacheKey
	}
	return out
}