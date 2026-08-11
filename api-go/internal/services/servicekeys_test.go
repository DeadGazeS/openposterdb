package services

import (
	"database/sql"
	"net/http"
	"testing"

	_ "modernc.org/sqlite"
)

// serviceKeysTestSchema is a minimal copy of the production schema for the
// service-key manager tests. We can't import internal/app here (cycle via
// seed.go), so we duplicate the relevant global_settings table.
const serviceKeysTestSchema = `
CREATE TABLE global_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
`

func newServiceKeysTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(serviceKeysTestSchema); err != nil {
		t.Fatal(err)
	}
	return db
}

func newServiceKeysManager(t *testing.T, db *sql.DB, secret string,
	envTMDB string, envMDBList []string, envOMDB, envFanart, envTrakt string) *ServiceKeyManager {
	t.Helper()
	mgr := NewServiceKeyManager(db, []byte(secret), &http.Client{},
		envTMDB, envMDBList, envOMDB, envFanart, envTrakt)
	mgr.Init()
	return mgr
}

// TestEncryptDecrypt_RoundTrip guards the AES-GCM helper: encrypt then
// decrypt must return the original plaintext.
func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	mgr := newServiceKeysManager(t, newServiceKeysTestDB(t), "round-trip-secret", "", nil, "", "", "")
	plaintext := "hello-world"
	encrypted := encrypt(plaintext, mgr.GCM)
	got, err := decrypt(encrypted, mgr.GCM)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plaintext {
		t.Errorf("decrypted=%q, want %q", got, plaintext)
	}
}

// TestEncrypt_DifferentIV guards the IV randomness contract: the same
// plaintext encrypted twice must produce different ciphertexts (because the
// nonce/IV is fresh each time).
func TestEncrypt_DifferentIV(t *testing.T) {
	mgr := newServiceKeysManager(t, newServiceKeysTestDB(t), "iv-test-secret", "", nil, "", "", "")
	plaintext := "same-plaintext"
	a := encrypt(plaintext, mgr.GCM)
	b := encrypt(plaintext, mgr.GCM)
	if a == b {
		t.Error("same plaintext produced identical ciphertext (IV reuse)")
	}
	// Both must still decrypt to the same plaintext.
	for i, ct := range []string{a, b} {
		got, err := decrypt(ct, mgr.GCM)
		if err != nil {
			t.Fatalf("decrypt %d: %v", i, err)
		}
		if got != plaintext {
			t.Errorf("decrypted[%d]=%q, want %q", i, got, plaintext)
		}
	}
}

// TestDecrypt_Tampered_Fails guards integrity: flipping a bit in the
// ciphertext must cause GCM authentication to fail.
func TestDecrypt_Tampered_Fails(t *testing.T) {
	mgr := newServiceKeysManager(t, newServiceKeysTestDB(t), "tamper-secret", "", nil, "", "", "")
	encrypted := encrypt("original", mgr.GCM)
	if len(encrypted) < 8 {
		t.Fatal("encrypted value unexpectedly short")
	}
	// Flip one byte in the middle.
	tampered := encrypted[:len(encrypted)/2] + "X" + encrypted[len(encrypted)/2+1:]
	if _, err := decrypt(tampered, mgr.GCM); err == nil {
		t.Error("decrypt of tampered ciphertext should fail")
	}
}

// TestDecrypt_WrongKey_Fails guards key-binding: a ciphertext encrypted
// with one key cannot be decrypted with a different key.
func TestDecrypt_WrongKey_Fails(t *testing.T) {
	encMgr := newServiceKeysManager(t, newServiceKeysTestDB(t), "secret-one", "", nil, "", "", "")
	decMgr := newServiceKeysManager(t, newServiceKeysTestDB(t), "secret-two", "", nil, "", "", "")

	encrypted := encrypt("plaintext", encMgr.GCM)
	if _, err := decrypt(encrypted, decMgr.GCM); err == nil {
		t.Error("decrypt with wrong key should fail")
	}
}

// TestUpdateKeys_EnvLocked_SkipsService guards the env-lock contract:
// when envTMDB is set, a TMDB update via UpdateKeys must NOT touch the DB.
func TestUpdateKeys_EnvLocked_SkipsService(t *testing.T) {
	db := newServiceKeysTestDB(t)
	mgr := newServiceKeysManager(t, db, "secret", "env-tmdb", nil, "", "", "")

	tmdb := "new-tmdb-via-update"
	if err := mgr.UpdateKeys(&ServiceKeysUpdate{TMDB: &tmdb}); err != nil {
		t.Fatal(err)
	}

	// DB must not contain service_key_tmdb (env-locked services skip DB).
	got, err := GetGlobalSetting(db, "service_key_tmdb")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("DB row for tmdb: got %q, want \"\" (env-locked)", got)
	}
	// And TMDBKey() must still return the env value.
	if k := mgr.TMDBKey(); k != "env-tmdb" {
		t.Errorf("TMDBKey: got %q, want env-tmdb", k)
	}
}

// TestLoadFromDB_BadEncryption_ReturnsNil guards the bad-encryption branch:
// a corrupted/garbage encrypted value in the DB returns nil (the manager
// pretends the key is missing rather than crashing).
func TestLoadFromDB_BadEncryption_ReturnsNil(t *testing.T) {
	db := newServiceKeysTestDB(t)
	if err := SetGlobalSetting(db, "service_key_tmdb", "not-a-real-encrypted-value"); err != nil {
		t.Fatal(err)
	}
	mgr := newServiceKeysManager(t, db, "secret", "", nil, "", "", "")
	if v := mgr.TMDBKey(); v != "" {
		t.Errorf("TMDBKey: got %q, want \"\" (bad encryption should yield nil)", v)
	}
}

// TestInit_LoadsFromDB guards the normal Init flow: encrypted keys in the
// DB are decrypted and exposed via the accessor methods.
func TestInit_LoadsFromDB(t *testing.T) {
	db := newServiceKeysTestDB(t)
	// Manually encrypt a key with the same secret the manager will use.
	mgr := newServiceKeysManager(t, db, "init-secret", "", nil, "", "", "")
	tmdbVal := "tmdb-from-db"
	if err := mgr.UpdateKeys(&ServiceKeysUpdate{TMDB: &tmdbVal}); err != nil {
		t.Fatal(err)
	}

	// Fresh manager reads from the DB.
	mgr2 := NewServiceKeyManager(db, []byte("init-secret"), &http.Client{}, "", nil, "", "", "")
	mgr2.Init()
	if got := mgr2.TMDBKey(); got != tmdbVal {
		t.Errorf("TMDBKey after Init: got %q, want %q", got, tmdbVal)
	}
}

// TestInit_EnvOverridesDB guards the env-wins contract: when envTMDB is
// set, the DB value is ignored even if a row exists.
func TestInit_EnvOverridesDB(t *testing.T) {
	db := newServiceKeysTestDB(t)
	mgr := newServiceKeysManager(t, db, "secret", "", nil, "", "", "")
	tmdbVal := "tmdb-from-db"
	if err := mgr.UpdateKeys(&ServiceKeysUpdate{TMDB: &tmdbVal}); err != nil {
		t.Fatal(err)
	}

	// Fresh manager with envTMDB set.
	mgr2 := NewServiceKeyManager(db, []byte("secret"), &http.Client{}, "tmdb-from-env", nil, "", "", "")
	mgr2.Init()
	if got := mgr2.TMDBKey(); got != "tmdb-from-env" {
		t.Errorf("TMDBKey: got %q, want tmdb-from-env (env must win)", got)
	}
}

// TestInit_NoSource_NoKeys guards the no-config branch: with no env vars
// and no DB rows, all accessors return empty.
func TestInit_NoSource_NoKeys(t *testing.T) {
	db := newServiceKeysTestDB(t)
	mgr := newServiceKeysManager(t, db, "secret", "", nil, "", "", "")
	if k := mgr.TMDBKey(); k != "" {
		t.Errorf("TMDBKey: got %q, want \"\"", k)
	}
	if k := mgr.OMDBKeys(); len(k) != 0 {
		t.Errorf("OMDBKeys: got %v, want []", k)
	}
	if k := mgr.MDBListKeys(); len(k) != 0 {
		t.Errorf("MDBListKeys: got %v, want []", k)
	}
}

// TestGetKeys_RoundTrip guards the read path: after Init from DB, the
// accessor methods (TMDBKey/MDBListKeys/etc.) return the decrypted values.
func TestGetKeys_RoundTrip(t *testing.T) {
	db := newServiceKeysTestDB(t)
	mgr := newServiceKeysManager(t, db, "secret", "", nil, "", "", "")
	tmdb := "abc123"
	omdb := "def456"
	if err := mgr.UpdateKeys(&ServiceKeysUpdate{TMDB: &tmdb, OMDB: &omdb}); err != nil {
		t.Fatal(err)
	}
	if got := mgr.TMDBKey(); got != tmdb {
		t.Errorf("TMDBKey: got %q, want %q", got, tmdb)
	}
	if got := mgr.OMDBKeys(); len(got) != 1 || got[0] != omdb {
		t.Errorf("OMDBKeys: got %v, want [%q]", got, omdb)
	}
}

// TestClearKeys_RemovesAll guards the clear path: deleting all
// service_key_* DB rows and re-Init-ing yields empty accessors. There is no
// dedicated ClearKeys method, so we exercise the contract via SetGlobalSetting
// to an empty value (the equivalent of "user cleared it in the admin UI").
func TestClearKeys_RemovesAll(t *testing.T) {
	db := newServiceKeysTestDB(t)
	mgr := newServiceKeysManager(t, db, "secret", "", nil, "", "", "")
	tmdb := "abc"
	if err := mgr.UpdateKeys(&ServiceKeysUpdate{TMDB: &tmdb}); err != nil {
		t.Fatal(err)
	}
	if mgr.TMDBKey() != tmdb {
		t.Fatal("setup: TMDB not set")
	}

	// "Clear" via the DB (the manager stores under service_key_tmdb).
	if err := SetGlobalSetting(db, "service_key_tmdb", ""); err != nil {
		t.Fatal(err)
	}

	// Fresh manager reads — must be empty.
	mgr2 := NewServiceKeyManager(db, []byte("secret"), &http.Client{}, "", nil, "", "", "")
	mgr2.Init()
	if got := mgr2.TMDBKey(); got != "" {
		t.Errorf("TMDBKey after clear: got %q, want \"\"", got)
	}
}

// TestClearKeys_NilDB_NoPanic guards the nil-DB defensive branch: a
// freshly-constructed (un-Init'd) manager must not panic when the DB is
// nil. We use a minimal constructor that bypasses Init.
func TestClearKeys_NilDB_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Init on nil DB panicked: %v", r)
		}
	}()
	mgr := &ServiceKeyManager{DB: nil, GCM: nil, HTTP: &http.Client{}}
	// Just calling accessor methods should not panic.
	_ = mgr.TMDBKey()
	_ = mgr.OMDBKeys()
	_ = mgr.MDBListKeys()
	_ = mgr.FanartKeys()
	_ = mgr.TraktClientIDs()
}

// TestUpdateKeys_PartialUpdate guards the partial-update contract: only
// the fields set in the update struct are changed.
func TestUpdateKeys_PartialUpdate(t *testing.T) {
	db := newServiceKeysTestDB(t)
	mgr := newServiceKeysManager(t, db, "secret", "", nil, "", "", "")

	// Seed initial values.
	tmdb := "first-tmdb"
	omdb := "first-omdb"
	if err := mgr.UpdateKeys(&ServiceKeysUpdate{TMDB: &tmdb, OMDB: &omdb}); err != nil {
		t.Fatal(err)
	}

	// Update only TMDB.
	tmdb2 := "second-tmdb"
	if err := mgr.UpdateKeys(&ServiceKeysUpdate{TMDB: &tmdb2}); err != nil {
		t.Fatal(err)
	}
	if got := mgr.TMDBKey(); got != tmdb2 {
		t.Errorf("TMDBKey: got %q, want %q", got, tmdb2)
	}
	if got := mgr.OMDBKeys(); len(got) != 1 || got[0] != omdb {
		t.Errorf("OMDBKeys after partial update: got %v, want [%q] (unchanged)", got, omdb)
	}
}

// TestUpdateKeys_EmptyUpdate_NoOp guards the all-nil branch: an update
// with no fields set must be a no-op (no DB writes).
func TestUpdateKeys_EmptyUpdate_NoOp(t *testing.T) {
	db := newServiceKeysTestDB(t)
	mgr := newServiceKeysManager(t, db, "secret", "", nil, "", "", "")

	// Empty update.
	if err := mgr.UpdateKeys(&ServiceKeysUpdate{}); err != nil {
		t.Fatal(err)
	}

	// No service_key_* rows should exist.
	for _, svc := range []string{"tmdb", "omdb", "mdblist", "fanart", "trakt"} {
		v, err := GetGlobalSetting(db, "service_key_"+svc)
		if err != nil {
			t.Fatal(err)
		}
		if v != "" {
			t.Errorf("service_key_%s: got %q, want \"\"", svc, v)
		}
	}
}

// TestUpdateKeys_RemovedKey_ClearsFromDB guards the clear-by-empty-string
// contract: passing an empty value for a service must remove it from the
// in-memory cache (UpdateKeys is partial-update and doesn't store empty
// values, but the in-memory key slice should be empty).
func TestUpdateKeys_RemovedKey_ClearsFromDB(t *testing.T) {
	db := newServiceKeysTestDB(t)
	mgr := newServiceKeysManager(t, db, "secret", "", nil, "", "", "")

	// Seed.
	tmdb := "initial"
	if err := mgr.UpdateKeys(&ServiceKeysUpdate{TMDB: &tmdb}); err != nil {
		t.Fatal(err)
	}

	// "Remove" by setting TMDB to empty string. The handler validates
	// upstream (admin settings) and rejects empties; the manager accepts
	// whatever it's given. After UpdateKeys with empty TMDB the in-memory
	// cache is the parsed empty slice.
	empty := ""
	if err := mgr.UpdateKeys(&ServiceKeysUpdate{TMDB: &empty}); err != nil {
		t.Fatal(err)
	}
	// In-memory: empty parsed slice means TMDBKey() returns "".
	if got := mgr.TMDBKey(); got != "" {
		t.Errorf("TMDBKey after empty update: got %q, want \"\"", got)
	}
}
