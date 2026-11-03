package handlers

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Tokens are only issued as HS256 with an expiry; anything else is rejected.
func TestParseJWT_PinsMethodAndRequiresExpiry(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	sign := func(m jwt.SigningMethod, c jwt.MapClaims) string {
		s, err := jwt.NewWithClaims(m, c).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	exp := time.Now().Add(time.Hour).Unix()
	if _, err := ParseJWT(sign(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "admin", "username": "admin", "exp": exp}), key); err != nil {
		t.Fatalf("valid HS256 token rejected: %v", err)
	}
	if _, err := ParseJWT(sign(jwt.SigningMethodHS384, jwt.MapClaims{"sub": "admin", "username": "admin", "exp": exp}), key); err == nil {
		t.Error("HS384 token must be rejected")
	}
	if _, err := ParseJWT(sign(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "admin", "username": "admin"}), key); err == nil {
		t.Error("token without exp must be rejected")
	}
}
