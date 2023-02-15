package utils

import (
	"regexp"

	"github.com/google/uuid"
)

func CompileRegex(regex string) *regexp.Regexp {
	return regexp.MustCompile(regex)
}

func GetByRegex(re *regexp.Regexp, content string) string {
	matches := re.FindStringSubmatch(content)
	if len(matches) > 0 {
		return matches[1]
	}
	return ""
}

func CreateUuid() string {
	id := uuid.New()
	return id.String()
}
