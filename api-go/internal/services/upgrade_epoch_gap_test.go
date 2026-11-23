package services

import (
	"database/sql"
	"strconv"
	"testing"
)

// A render-epoch sweep that fails is not recorded (runOnceCtx inserts the
// upgrades row only after f succeeded), so the next boot retries it; once a
// run succeeds the epoch is recorded and the old-epoch render is gone.
func TestRunUpgrades_FailedEpochSweepNotRecorded(t *testing.T) {
	if RenderEpoch <= 0 {
		t.Skip("no render epoch")
	}
	db := upgradeTestDB(t)
	cacheDir := t.TempDir()
	epochName := "render_epoch_" + strconv.Itoa(RenderEpoch)

	// Every other upgrade done; the epoch sweep still pending.
	if err := RunUpgrades(db, cacheDir, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DELETE FROM upgrades WHERE name = ?", epochName); err != nil {
		t.Fatal(err)
	}

	// The sweep's first query (SELECT cache_key FROM image_meta) fails.
	if _, err := db.Exec("ALTER TABLE image_meta RENAME TO image_meta_hidden"); err != nil {
		t.Fatal(err)
	}
	if err := RunUpgrades(db, cacheDir, false); err == nil {
		t.Fatal("RunUpgrades succeeded with the epoch sweep failing")
	}
	if epochRecorded(t, db, epochName) {
		t.Fatal("failed epoch sweep was recorded; it would never be retried")
	}

	// Next boot: the sweep works, removes the old-epoch row and is recorded.
	if _, err := db.Exec("ALTER TABLE image_meta_hidden RENAME TO image_meta"); err != nil {
		t.Fatal(err)
	}
	oldKey := "imdb/tt1@mil.sh.li.dd.shr.ba80.zm" // epoch 0: no .re<n> segment
	if _, err := db.Exec(`INSERT INTO image_meta (cache_key, created_at, updated_at, image_type) VALUES (?, 1, 1, 'p')`, oldKey); err != nil {
		t.Fatal(err)
	}
	if err := RunUpgrades(db, cacheDir, false); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !epochRecorded(t, db, epochName) {
		t.Fatal("successful epoch sweep not recorded")
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM image_meta WHERE cache_key = ?", oldKey).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("old-epoch render survived the retried sweep")
	}
}

func epochRecorded(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM upgrades WHERE name = ?", name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}
