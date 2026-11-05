package service

import (
	"regexp"
	"strings"
)

// matchesKBPattern mirrors packages/cookie-sdk/src/scripts.ts's own matchesPattern exactly: a plain
// substring match by default, or a /regex/-wrapped pattern when cookie.cookie_kb's own data wants one. Kept
// as its own small pure function, the same "DOM-free functions get a direct unit test" split CON-02's own
// frontend scripts.ts already uses.
func matchesKBPattern(name, pattern string) bool {
	if len(pattern) > 1 && pattern[0] == '/' && pattern[len(pattern)-1] == '/' {
		re, err := regexp.Compile(pattern[1 : len(pattern)-1])
		if err != nil {
			return false
		}
		return re.MatchString(name)
	}
	return pattern != "" && strings.Contains(name, pattern)
}
