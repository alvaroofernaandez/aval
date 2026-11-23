package i18n

import (
	"errors"
	"testing"
)

func TestJoinRejectsDuplicateKeys(t *testing.T) {
	_, err := joinParts(catalog{"a": {other: "one"}}, catalog{"a": {other: "two"}})
	if !errors.Is(err, errDuplicateKey) {
		t.Fatalf("joinParts with a key in two parts: err = %v, want errDuplicateKey", err)
	}
}

func TestJoinMergesDisjointParts(t *testing.T) {
	got, err := joinParts(catalog{"a": {other: "A"}}, catalog{"b": {other: "B"}})
	if err != nil || len(got) != 2 || got["b"].other != "B" {
		t.Fatalf("joinParts = %v, %v; want both keys", got, err)
	}
}
