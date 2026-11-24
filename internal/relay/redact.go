package relay

import "regexp"

const redacted = "[redacted]"

// credentialShapes are the credential formats masked in capsule text. The
// relay owns this table (D1 of odd/tasks/relay.md): agent/prepush holds
// employer names, not credential shapes, and a leaf cannot import it.
var credentialShapes = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{40,}`),
	regexp.MustCompile(`\bsk-(?:ant-|proj-)?[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/-]{20,}=*`),
}

// Redact masks credential shapes in s and leaves the rest byte-identical.
func Redact(s string) string {
	for _, re := range credentialShapes {
		s = re.ReplaceAllString(s, redacted)
	}
	return s
}
