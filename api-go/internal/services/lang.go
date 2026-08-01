package services

import "strings"

func LangBase(lang string) string {
	parts := strings.SplitN(lang, "-", 2)
	return parts[0]
}

func LangRegion(lang string) string {
	parts := strings.SplitN(lang, "-", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}
