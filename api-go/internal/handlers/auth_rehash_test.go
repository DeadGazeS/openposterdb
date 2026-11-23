package handlers

import (
	"testing"

	"openposterdb/internal/services"
)

// TestLogin_RustPHCHashIsUpgraded: an admin created by the Rust version
// (PHC argon2id string) can log in, and the stored hash is rewritten in the
// current format so the PHC path is only needed once.
func TestLogin_RustPHCHashIsUpgraded(t *testing.T) {
	db := authTestDB(t)
	// Same vector as services/users_phc_test.go (password "hunter2-rust").
	const phc = "$argon2id$v=19$m=19456,t=2,p=1$MDEyMzQ1Njc4OWFiY2RlZg$uQK7PTS7RvQEeuHXEGpwFtFS3a2wz+9jZdD72dVq39A"
	if _, err := db.Exec("INSERT INTO admin_users (username, password_hash) VALUES ('admin', ?)", phc); err != nil {
		t.Fatal(err)
	}

	if status, _, _ := LoginHandler(authContext(), db, testJWTSecret, false, "admin", "wrong"); status != 401 {
		t.Fatalf("wrong password: status %d, want 401", status)
	}
	var stored string
	if err := db.QueryRow("SELECT password_hash FROM admin_users WHERE username = 'admin'").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != phc {
		t.Fatal("hash must not change on a failed login")
	}

	if status, _, _ := LoginHandler(authContext(), db, testJWTSecret, false, "admin", "hunter2-rust"); status != 200 {
		t.Fatalf("correct password: status %d, want 200", status)
	}
	if err := db.QueryRow("SELECT password_hash FROM admin_users WHERE username = 'admin'").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if services.NeedsRehash(stored) {
		t.Fatalf("hash still in PHC format after login: %q", stored)
	}
	if ok, err := services.VerifyPassword("hunter2-rust", stored); err != nil || !ok {
		t.Fatalf("re-hashed password does not verify: ok=%v err=%v", ok, err)
	}
	if status, _, _ := LoginHandler(authContext(), db, testJWTSecret, false, "admin", "hunter2-rust"); status != 200 {
		t.Fatalf("second login: status %d, want 200", status)
	}
}
