package app_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"openposterdb/internal/app"
	"openposterdb/internal/services"
)

func openLockTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := app.OpenDatabase(filepath.Join(dir, "lock.db")) // production DSN + pool
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := app.RunSchema(db, app.SchemaSQL); err != nil {
		t.Fatal(err)
	}
	if err := app.RunMigrations(db, app.Migrations); err != nil {
		t.Fatal(err)
	}
	if err := services.RunUpgrades(db, dir, true); err != nil {
		t.Fatal(err)
	}
	return db
}

func isLocked(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "locked") || strings.Contains(s, "busy")
}

// CreateFirstAdminUserCtx reads (COUNT) then writes (INSERT) in one
// transaction. Racing setups must end with exactly one admin and clean
// "Setup already completed" errors — never "database is locked".
func TestDBLock_ConcurrentFirstAdminSetup(t *testing.T) {
	for round := 0; round < 10; round++ {
		db := openLockTestDB(t)
		var ok, already, locked atomic.Int32
		var other []string
		var mu sync.Mutex
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				_, err := services.CreateFirstAdminUserCtx(context.Background(), db, fmt.Sprintf("admin%d", i), "hash")
				switch {
				case err == nil:
					ok.Add(1)
				case strings.Contains(err.Error(), "Setup already completed"):
					already.Add(1)
				case isLocked(err):
					locked.Add(1)
				default:
					mu.Lock()
					other = append(other, err.Error())
					mu.Unlock()
				}
			}(i)
		}
		close(start)
		wg.Wait()
		if locked.Load() > 0 || ok.Load() != 1 || len(other) > 0 {
			t.Fatalf("round %d: ok=%d already=%d locked=%d other=%v", round, ok.Load(), already.Load(), locked.Load(), other)
		}
	}
}

// Every write path the server runs concurrently, at once, against the real
// pool: none may fail with "database is locked".
func TestDBLock_MixedWriteStress(t *testing.T) {
	db := openLockTestDB(t)
	ctx := context.Background()
	app.SeedAdminIfNeeded(db, "admin", "stresspassword")
	keyID, err := services.CreateAPIKeyCtx(ctx, db, "k", "h0", "p", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	var locked atomic.Int32
	var firstErr atomic.Value
	record := func(err error) {
		if isLocked(err) {
			locked.Add(1)
			firstErr.CompareAndSwap(nil, err.Error())
		}
	}
	var wg sync.WaitGroup
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 150; i++ {
				key := fmt.Sprintf("imdb/tt%07d", (w*150+i)%400)
				switch i % 7 {
				case 0:
					record(services.UpsertImageMetaCtx(ctx, db, key, nil, "p", "tmdb:movie-1", nil))
				case 1:
					record(services.SetGlobalSettingsBatchCtx(ctx, db, map[string]string{"lang": "en", fmt.Sprintf("x%d", w): "1"}))
				case 2:
					record(services.RemoveGlobalSettingsCtx(ctx, db, []string{fmt.Sprintf("x%d", w)}))
				case 3:
					record(services.UpsertAPIKeySettingsCtx(ctx, db, &services.APIKeySettings{APIKeyID: keyID, Lang: "de"}))
				case 4:
					services.TouchImageAccessCtx(ctx, db, key)
					services.FlushImageAccess(db)
				case 5:
					_, err := services.GetGlobalSettingsCtx(ctx, db)
					record(err)
				case 6:
					_, _, err := services.ListImageMetaByKindCtx(ctx, db, "p", "created_at", "desc", 1, 50)
					record(err)
				}
			}
		}(w)
	}
	wg.Wait()
	if n := locked.Load(); n > 0 {
		t.Fatalf("%d operations failed with a lock error, first: %v", n, firstErr.Load())
	}
}
