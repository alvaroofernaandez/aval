package relay

import (
	"strings"
	"testing"
)

func TestRedact_MasksCredentialShapes(t *testing.T) {
	secrets := []string{
		"AKIAIOSFODNN7EXAMPLE",
		"ghp_abcdefghijklmnopqrstuvwxyz0123456789",
		"github_pat_11ABCDEFG0abcdefghijkl_mnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUV",
		"sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123",
		"sk-proj-abcdefghijklmnopqrstuvwxyz0123",
		"xox" + "b-123456789012-1234567890123-abcdefghijklmnopqrstuvwx", // split: no literal for secret scanners
		"glpat" + "-abcdefghijklmnopqrst",
		"Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abcdefghij",
		"-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----",
	}
	for _, s := range secrets {
		got := Redact("before " + s + " after")
		if strings.Contains(got, s) || !strings.Contains(got, redacted) {
			t.Errorf("not masked: %q -> %q", s, got)
		}
		if !strings.HasPrefix(got, "before ") || !strings.HasSuffix(got, " after") {
			t.Errorf("text around the secret changed: %q", got)
		}
	}
	plain := "go test ./internal/relay/ -run SelectTail"
	if got := Redact(plain); got != plain {
		t.Errorf("plain text changed: %q -> %q", plain, got)
	}
}
