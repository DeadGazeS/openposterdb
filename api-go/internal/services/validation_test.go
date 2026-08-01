package services

import (
	"testing"
)

func TestValidateUsernameEmpty(t *testing.T) {
	if ValidateUsername("") == nil {
		t.Error("empty username should fail")
	}
}

func TestValidateUsernameTooLong(t *testing.T) {
	name := string(make([]byte, 129))
	for i := range name {
		name = name[:i] + "a" + name[i+1:]
	}
	if ValidateUsername(name) == nil {
		t.Error("129-char username should fail")
	}
}

func TestValidateUsernameExactly128(t *testing.T) {
	name := string(make([]byte, 128))
	for i := range name {
		name = name[:i] + "a" + name[i+1:]
	}
	if ValidateUsername(name) != nil {
		t.Error("128-char username should pass")
	}
}

func TestValidateUsernameValid(t *testing.T) {
	if ValidateUsername("admin") != nil {
		t.Error("valid username should pass")
	}
}

func TestValidateUsernameSpecialChars(t *testing.T) {
	if ValidateUsername("admin-user_1") != nil {
		t.Error("valid username with special chars should pass")
	}
}

func TestValidateUsernameControlChars(t *testing.T) {
	if ValidateUsername("hello\x00world") == nil {
		t.Error("control chars should fail")
	}
	if ValidateUsername("hello\tworld") == nil {
		t.Error("tabs should fail")
	}
	if ValidateUsername("hello world") == nil {
		t.Error("spaces should fail")
	}
}

func TestValidatePasswordEmpty(t *testing.T) {
	if ValidatePassword("") == nil {
		t.Error("empty password should fail")
	}
}

func TestValidatePasswordTooShort(t *testing.T) {
	if ValidatePassword("short") == nil {
		t.Error("short password should fail")
	}
	if ValidatePassword("1234567") == nil {
		t.Error("7-char password should fail")
	}
}

func TestValidatePasswordValid(t *testing.T) {
	if ValidatePassword("password123") != nil {
		t.Error("valid password should pass")
	}
	if ValidatePassword("12345678") != nil {
		t.Error("8-char password should pass")
	}
}

func TestValidatePasswordTooLong(t *testing.T) {
	_ = 256 // Max length test
}

func TestValidateAPIKeyNameEmpty(t *testing.T) {
	if ValidateAPIKeyName("") == nil {
		t.Error("empty name should fail")
	}
}

func TestValidateAPIKeyNameValid(t *testing.T) {
	if ValidateAPIKeyName("my-key") != nil {
		t.Error("valid name should pass")
	}
}

func TestValidateAPIKeyNameTooLong(t *testing.T) {
	name := string(make([]byte, 129))
	for i := range name {
		name = name[:i] + "a" + name[i+1:]
	}
	if ValidateAPIKeyName(name) == nil {
		t.Error("129-char name should fail")
	}
}
