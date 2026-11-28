package usage

import (
	"slices"
	"time"
)

// Window lengths of the plans' usage limits.
const (
	BlockLength = 5 * time.Hour
	WeekLength  = 7 * 24 * time.Hour
)

// Block is a 5 h billing block, with ccusage's semantics: it starts at the
// first message, floored to the hour in UTC, and ends five hours later; a
// message at or after the end, or five hours after the previous message,
// opens the next block.
type Block struct {
	Start, End, Last time.Time
	Row
}

// Blocks groups entries into blocks, oldest first.
func Blocks(entries []Entry) []Block {
	sorted := slices.Clone(entries)
	slices.SortStableFunc(sorted, func(a, b Entry) int { return a.Time.Compare(b.Time) })
	var out []Block
	var cur []Entry
	flush := func() {
		if len(cur) == 0 {
			return
		}
		b := Block{Start: cur[0].Time.UTC().Truncate(time.Hour), Last: cur[len(cur)-1].Time}
		b.End = b.Start.Add(BlockLength)
		b.Row = Sum(Aggregate(cur, ByDay, time.UTC))
		b.Key = b.Start.Format(time.RFC3339)
		out = append(out, b)
		cur = nil
	}
	var start, last time.Time
	for _, e := range sorted {
		if len(cur) > 0 && (!e.Time.Before(start.Add(BlockLength)) || e.Time.Sub(last) >= BlockLength) {
			flush()
		}
		if len(cur) == 0 {
			start = e.Time.UTC().Truncate(time.Hour)
		}
		cur = append(cur, e)
		last = e.Time
	}
	flush()
	return out
}

// Active is the block that is still open at now: now is before its end and
// less than five hours after its last message.
func Active(blocks []Block, now time.Time) (Block, bool) {
	if len(blocks) == 0 {
		return Block{}, false
	}
	b := blocks[len(blocks)-1]
	if now.Before(b.End) && now.Sub(b.Last) < BlockLength {
		return b, true
	}
	return Block{}, false
}

// Remaining is the time until the block closes.
func (b Block) Remaining(now time.Time) time.Duration { return max(0, b.End.Sub(now)) }

// InBlock selects the entries of a block.
func InBlock(entries []Entry, b Block) []Entry {
	return slices.DeleteFunc(slices.Clone(entries), func(e Entry) bool {
		return e.Time.Before(b.Start) || !e.Time.Before(b.End)
	})
}

// InLast selects the entries after now-d and up to now; an entry stamped later
// than now is ignored.
func InLast(entries []Entry, d time.Duration, now time.Time) []Entry {
	from := now.Add(-d)
	return slices.DeleteFunc(slices.Clone(entries), func(e Entry) bool {
		return !e.Time.After(from) || e.Time.After(now)
	})
}
