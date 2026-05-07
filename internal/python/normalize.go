package python

import (
	"regexp"
	"strings"
)

var nameSeparatorPattern = regexp.MustCompile(`[-_.]+`)

func NormalizeName(value string) string {
	return nameSeparatorPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-")
}
