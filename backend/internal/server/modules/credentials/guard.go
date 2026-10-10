package credentials

import (
	"regexp"
	"strings"
)

// The ledger keeps facts about secrets, never the secret itself. A text field
// that looks like a key or a token is refused, so a paste into the wrong box
// does not end up in the database or in a backup.

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY`),
	regexp.MustCompile(`\b(?:sk|pk|rk)-[A-Za-z0-9_-]{16,}`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`\bxc_[0-9a-fA-F]{20,}`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{30,}`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.`),
}

var longWord = regexp.MustCompile(`[A-Za-z0-9_+=-]{32,}`)

// looksLikeSecret reports whether s contains something that has the shape of a
// real secret. A long word counts only when it mixes upper case, lower case
// and digits, so slugs and UUIDs pass. A SSH fingerprint after "SHA256:" or
// "MD5:" is a public value and passes too.
func looksLikeSecret(s string) bool {
	for _, re := range secretPatterns {
		if re.MatchString(s) {
			return true
		}
	}
	for _, loc := range longWord.FindAllStringIndex(s, -1) {
		before := strings.ToLower(s[:loc[0]])
		if strings.HasSuffix(before, "sha256:") || strings.HasSuffix(before, "md5:") {
			continue
		}
		var upper, lower, digit bool
		for _, r := range s[loc[0]:loc[1]] {
			switch {
			case r >= 'A' && r <= 'Z':
				upper = true
			case r >= 'a' && r <= 'z':
				lower = true
			case r >= '0' && r <= '9':
				digit = true
			}
		}
		if upper && lower && digit {
			return true
		}
	}
	return false
}
