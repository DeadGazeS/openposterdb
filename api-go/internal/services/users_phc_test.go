package services

import (
	"strings"
	"testing"
)

// rustPHCVector was produced independently of this package (Node's
// crypto.argon2Sync, argon2id v19) with the parameters the Rust argon2 crate
// uses by default (m=19456 KiB, t=2, p=1, 32-byte tag), password
// "hunter2-rust" and salt "0123456789abcdef".
const rustPHCVector = "$argon2id$v=19$m=19456,t=2,p=1$MDEyMzQ1Njc4OWFiY2RlZg$uQK7PTS7RvQEeuHXEGpwFtFS3a2wz+9jZdD72dVq39A"

func TestVerifyPassword_RustPHCHash(t *testing.T) {
	ok, err := VerifyPassword("hunter2-rust", rustPHCVector)
	if err != nil || !ok {
		t.Fatalf("correct password: ok=%v err=%v, want true/nil", ok, err)
	}
	ok, err = VerifyPassword("wrong", rustPHCVector)
	if err != nil || ok {
		t.Fatalf("wrong password: ok=%v err=%v, want false/nil", ok, err)
	}
}

func TestVerifyPassword_PHCWithoutVersion(t *testing.T) {
	noVersion := strings.Replace(rustPHCVector, "v=19$", "", 1)
	ok, err := VerifyPassword("hunter2-rust", noVersion)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v, want true/nil", ok, err)
	}
}

func TestVerifyPassword_MalformedPHC(t *testing.T) {
	for _, h := range []string{
		"$argon2id$v=16$m=19456,t=2,p=1$MDEyMzQ1Njc4OWFiY2RlZg$uQK7PTS7RvQEeuHXEGpwFtFS3a2wz+9jZdD72dVq39A",
		"$argon2id$v=19$m=19456,t=2$MDEyMzQ1Njc4OWFiY2RlZg$uQK7PTS7RvQEeuHXEGpwFtFS3a2wz+9jZdD72dVq39A",
		"$argon2id$v=19$m=99999999,t=2,p=1$MDEyMzQ1Njc4OWFiY2RlZg$uQK7PTS7RvQEeuHXEGpwFtFS3a2wz+9jZdD72dVq39A",
		"$argon2id$v=19$m=19456,t=2,p=1$!!!$uQK7PTS7RvQEeuHXEGpwFtFS3a2wz+9jZdD72dVq39A",
		"$argon2id$v=19$m=19456,t=2,p=1,x=3$MDEyMzQ1Njc4OWFiY2RlZg$uQK7PTS7RvQEeuHXEGpwFtFS3a2wz+9jZdD72dVq39A",
		"$argon2id$",
	} {
		if ok, err := VerifyPassword("hunter2-rust", h); err == nil || ok {
			t.Errorf("%q: ok=%v err=%v, want false + error", h, ok, err)
		}
	}
}

func TestNeedsRehash(t *testing.T) {
	if !NeedsRehash(rustPHCVector) {
		t.Error("PHC hash should need a rehash")
	}
	h, err := HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	if NeedsRehash(h) {
		t.Error("current-format hash should not need a rehash")
	}
}
