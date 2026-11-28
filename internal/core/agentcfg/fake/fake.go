// Package fake is the reference agentcfg.Adapter: it renders every element it
// supports as one file under <capability>/<id> and reports the rest. It is the
// adapter the contract suite is proven against, and the one tests use where a
// real runtime does not matter.
package fake

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
)

// Adapter is the fake runtime. Caps nil means every capability is Full.
type Adapter struct {
	Caps agentcfg.Capabilities
}

// New returns a fake that supports everything.
func New() *Adapter { return &Adapter{} }

// Name implements agentcfg.Adapter.
func (a *Adapter) Name() string { return "fake" }

// Capabilities implements agentcfg.Adapter.
func (a *Adapter) Capabilities() agentcfg.Capabilities {
	if a.Caps != nil {
		return a.Caps
	}
	all := agentcfg.Capabilities{}
	for _, c := range agentcfg.AllCapabilities {
		all[c] = agentcfg.Full
	}
	return all
}

// Render implements agentcfg.Adapter.
func (a *Adapter) Render(cfg agentcfg.Config) (agentcfg.Rendered, error) {
	var r agentcfg.Rendered
	caps := a.Capabilities()
	for _, e := range cfg.Elements() {
		if caps[e.Capability] == agentcfg.Unsupported {
			r.Warnings = append(r.Warnings, agentcfg.Warning{ID: e.ID, Capability: e.Capability, Reason: "the fake runtime has no " + string(e.Capability)})
			continue
		}
		sum := sha256.Sum256([]byte(e.ID))
		r.Files = append(r.Files, agentcfg.File{
			Path: string(e.Capability) + "/" + hex.EncodeToString(sum[:8]),
			Data: []byte(e.ID + "\n" + e.Hash + "\n"),
		})
		r.Projected = append(r.Projected, e.ID)
	}
	return r, nil
}

// Diff implements agentcfg.Adapter.
func (a *Adapter) Diff(target string, r agentcfg.Rendered) ([]agentcfg.Change, error) {
	return agentcfg.DiffFiles(target, r)
}

// Apply implements agentcfg.Adapter.
func (a *Adapter) Apply(target string, r agentcfg.Rendered, opt agentcfg.ApplyOptions) ([]agentcfg.Change, error) {
	return agentcfg.ApplyFiles(target, r, opt)
}
