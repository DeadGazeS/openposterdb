package services

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// With every hashing slot taken, a password check gives up after the bounded
// wait (ErrPasswordHashBusy) instead of queueing until the client leaves.
func TestArgon2SlotWait_Bounded(t *testing.T) {
	orig := passwordSlotWaitFor
	passwordSlotWaitFor = 50 * time.Millisecond
	t.Cleanup(func() { passwordSlotWaitFor = orig })
	for range cap(passwordHashSlots) {
		passwordHashSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for range cap(passwordHashSlots) {
			<-passwordHashSlots
		}
	})

	start := time.Now()
	_, err := VerifyPasswordCtx(context.Background(), "pw", "00:00")
	if !errors.Is(err, ErrPasswordHashBusy) {
		t.Fatalf("err %v, want ErrPasswordHashBusy", err)
	}
	if el := time.Since(start); el > time.Second {
		t.Errorf("gave up after %v, want about %v", el, passwordSlotWaitFor)
	}
	if _, err := HashPasswordCtx(context.Background(), "pw"); !errors.Is(err, ErrPasswordHashBusy) {
		t.Errorf("hash: err %v, want ErrPasswordHashBusy", err)
	}
}

func TestPasswordSlotWait_Constant(t *testing.T) {
	if passwordSlotWait <= 0 || passwordSlotWait > 10*time.Second {
		t.Errorf("passwordSlotWait %v, want a few seconds", passwordSlotWait)
	}
}

func adminTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE admin_users (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT (datetime('now')), prefs TEXT NOT NULL DEFAULT '{}')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestAdminUserExistsCached(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	if ok, err := AdminUserExistsCachedCtx(ctx, db, "admin"); err != nil || ok {
		t.Fatalf("missing user: %v %v, want false/nil", ok, err)
	}
	if _, err := CreateAdminUser(db, "admin", "x:y"); err != nil {
		t.Fatal(err)
	}
	// Missing users aren't cached: the new admin is seen at once.
	if ok, err := AdminUserExistsCachedCtx(ctx, db, "admin"); err != nil || !ok {
		t.Fatalf("existing user: %v %v, want true/nil", ok, err)
	}
	// A hit is served from the cache, without the database.
	db.Close()
	if ok, err := AdminUserExistsCachedCtx(ctx, db, "admin"); err != nil || !ok {
		t.Fatalf("cached user: %v %v, want true/nil", ok, err)
	}
	if _, err := AdminUserExistsCachedCtx(ctx, db, "other"); err == nil {
		t.Fatal("db error: want an error")
	}
}
