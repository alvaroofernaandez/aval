package relay

// Source is where a token reading comes from.
type Source int

const (
	SourceExact      Source = iota // the runtime reports the context tokens
	SourceProvider                 // the provider's usage of the last request
	SourceTranscript               // estimated from transcript characters
	SourceHeuristic                // a rough guess (MCP only)
)

// Estimator turns transcript characters into a Reading and learns a
// correction per model from observed exact readings.
type Estimator struct {
	ratio map[string]float64
}

// Error bounds per source, from E1 of spike #63 (in-sample: one person,
// one week, Claude Code, Opus) and the design assumptions of #64.
const (
	exactErrPct      = 0.02
	providerErrPct   = 0.05
	transcriptErrPct = 0.15
	unknownErrPct    = 0.50
	charsPerToken    = 1.75 // E1: pooled p95 13.7%; chars/4 underestimates 13 to 43%
)

// Known window sizes. An unknown model gets the smallest plausible window.
const smallestWindow = 200_000

// calibrationWeight is the weight of the newest exact reading.
const calibrationWeight = 0.5

// Transcript returns the reading for chars of transcript on top of floor.
// An empty model or window is unknown: the bound widens and the window is the
// smallest plausible one, so the relay fires early rather than late.
func (e *Estimator) Transcript(model string, floor, chars, window int) Reading {
	errPct := transcriptErrPct
	if model == "" || window <= 0 {
		errPct, window = unknownErrPct, smallestWindow
	}
	est := float64(floor) + float64(chars)/charsPerToken
	if r, ok := e.ratio[model]; ok {
		est *= r
	}
	return Reading{Tokens: int(est), Window: window, ErrPct: errPct, FloorTokens: floor, Source: SourceTranscript}
}

// Calibrate records an exact reading taken where the estimate was est, as an
// exponentially weighted ratio per model.
func (e *Estimator) Calibrate(model string, est, exact int) {
	if est <= 0 || exact <= 0 {
		return
	}
	if e.ratio == nil {
		e.ratio = map[string]float64{}
	}
	obs := float64(exact) / float64(est)
	if prev, ok := e.ratio[model]; ok {
		obs = calibrationWeight*obs + (1-calibrationWeight)*prev
	}
	e.ratio[model] = obs
}
