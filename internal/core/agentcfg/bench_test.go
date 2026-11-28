package agentcfg_test

import (
	"path/filepath"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/fake"
)

// BenchmarkLoadAndRender is the #8 budget: Load of the real repo plus a full
// Render in under 200 ms.
func BenchmarkLoadAndRender(b *testing.B) {
	repo, _ := filepath.Abs("../../..")
	a := fake.New()
	for b.Loop() {
		cfg, err := agentcfg.Load(repo)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := a.Render(cfg); err != nil {
			b.Fatal(err)
		}
	}
}
