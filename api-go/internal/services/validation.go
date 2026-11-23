package services

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// settingEnums lists the values a per-key setting field accepts (the
// Parse* functions turn anything else into the default). Aliases the
// parsers accept are listed too: "h"/"v" are the stored badge style
// defaults.
var settingEnums = map[string][]string{
	"_badge_style":     {"d", "lr", "rl", "tb", "bt", "h", "v"},
	"_label_style":     {"t", "i", "o", "h"},
	"_badge_shape":     {"r", "p"},
	"_badge_direction": {"d", "h", "v"},
	"image_source":     {"t", "f"},
	"poster_fit":       {"native", "cover", "pad", "blur"},
	"anime_artwork":    {"id", "kitsu", "mal"},
}

// ValidateSettingEnum checks an enum-valued settings field (by JSON name).
// Fields that aren't enums always pass.
func ValidateSettingEnum(field, value string) error {
	for suffix, allowed := range settingEnums {
		if field == suffix || strings.HasSuffix(field, suffix) && strings.HasPrefix(suffix, "_") {
			if !slices.Contains(allowed, value) {
				return fmt.Errorf("invalid %s %q: must be one of %s", field, value, strings.Join(allowed, ", "))
			}
			return nil
		}
	}
	return nil
}

// isControl reports whether a rune is a Unicode control character (Cc + Cf),
// matching Rust's char::is_control: C0 controls 0x00-0x1F, DEL 0x7F, and the
// C1 controls 0x80-0x9F. Format characters (Cf, e.g. U+200E) are included.
func isControl(c rune) bool {
	if c < 32 {
		return true
	}
	if c >= 0x7F && c <= 0x9F {
		return true
	}
	return c >= 0x200E && c <= 0x200F || (c >= 0x2028 && c <= 0x202E) || (c >= 0x2060 && c <= 0x206F)
}

func ValidateUsername(username string) error {
	if len(username) == 0 || len(username) > 128 {
		return fmt.Errorf("Invalid username: must be 1-128 characters and not contain whitespace/control characters")
	}
	for _, c := range username {
		// unicode.IsSpace also covers U+0085, U+00A0 and the U+2000 block
		// (en/em/thin spaces, U+3000 …), which looked like ASCII spaces.
		if c <= 32 || isControl(c) || unicode.IsSpace(c) || unicode.Is(unicode.Zs, c) {
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
		if isControl(c) {
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
		if isControl(c) {
			return fmt.Errorf("Invalid password: must be 8-256 characters and not contain control characters")
		}
	}
	return nil
}
