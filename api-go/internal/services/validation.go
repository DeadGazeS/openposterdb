package services

import "fmt"

func ValidateUsername(username string) error {
	if len(username) == 0 || len(username) > 128 {
		return fmt.Errorf("Invalid username: must be 1-128 characters and not contain whitespace/control characters")
	}
	for _, c := range username {
		if c <= 32 || c == 127 {
			return fmt.Errorf("Invalid username: must be 1-128 characters and not contain whitespace/control characters")
		}
	}
	return nil
}

func ValidateAPIKeyName(name string) error {
	if len(name) == 0 || len(name) > 128 {
		return fmt.Errorf("Invalid API key name: must be 1-128 characters and not contain control characters")
	}
	for _, c := range name {
		if c < 32 {
			return fmt.Errorf("Invalid API key name: must be 1-128 characters and not contain control characters")
		}
	}
	return nil
}

func ValidatePassword(password string) error {
	if len(password) < 8 || len(password) > 256 {
		return fmt.Errorf("Invalid password: must be 8-256 characters and not contain control characters")
	}
	for _, c := range password {
		if c < 32 {
			return fmt.Errorf("Invalid password: must be 8-256 characters and not contain control characters")
		}
	}
	return nil
}
