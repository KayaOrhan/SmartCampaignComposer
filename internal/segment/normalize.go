package segment

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

func NormalizeCity(value string) string {
	value = cases.Lower(language.Turkish).String(norm.NFC.String(strings.TrimSpace(value)))
	// The assignment uses both English and Turkish spellings of Istanbul.
	if value == "ıstanbul" || value == "istanbul" {
		return "istanbul"
	}
	return value
}
