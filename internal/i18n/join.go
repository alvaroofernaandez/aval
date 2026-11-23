package i18n

import (
	"errors"
	"fmt"
)

var errDuplicateKey = errors.New("i18n: key defined in two catalog parts")

// join merges a language's catalog parts. Each surface keeps its own part
// (core, ui, cli), so their translations live in separate files.
func join(parts ...catalog) catalog {
	c, err := joinParts(parts...)
	if err != nil {
		panic(err)
	}
	return c
}

func joinParts(parts ...catalog) (catalog, error) {
	out := catalog{}
	for _, p := range parts {
		for k, m := range p {
			if _, dup := out[k]; dup {
				return nil, fmt.Errorf("%w: %s", errDuplicateKey, k)
			}
			out[k] = m
		}
	}
	return out, nil
}
