package usage

import (
	"cmp"
	"maps"
	"slices"
	"time"
)

// By is the grouping of a report.
type By string

const (
	ByProject  By = "project"
	BySession  By = "session"
	ByDay      By = "day"
	ByConsumer By = "consumer"
)

// Row is one group of a report.
type Row struct {
	Key      string
	Tokens   Tokens
	Messages int
	// Cost is the sum over the responses that have one; Unpriced counts those
	// that have none (a model the table does not know), so a total never
	// passes a partial sum off as whole.
	Cost     Cost
	Unpriced int
	// Models lists the models of unpriced responses, sorted.
	UnpricedModels []string
}

// Aggregate groups entries. A day is a calendar day in loc. Rows come back by
// day ascending, or by total tokens descending then key for the other groupings.
func Aggregate(entries []Entry, by By, loc *time.Location) []Row {
	idx := map[string]*Row{}
	unpriced := map[string]map[string]bool{}
	for _, e := range entries {
		k := groupKey(e, by, loc)
		r := idx[k]
		if r == nil {
			r = &Row{Key: k}
			idx[k] = r
			unpriced[k] = map[string]bool{}
		}
		r.Tokens = r.Tokens.Add(e.Tokens)
		r.Messages++
		if c, ok := EntryCost(e); ok {
			r.Cost += c
		} else {
			r.Unpriced++
			unpriced[k][e.Model] = true
		}
	}
	rows := make([]Row, 0, len(idx))
	for k, r := range idx {
		r.UnpricedModels = slices.Sorted(maps.Keys(unpriced[k]))
		rows = append(rows, *r)
	}
	slices.SortFunc(rows, func(a, b Row) int {
		if by != ByDay && a.Tokens.Total() != b.Tokens.Total() {
			return cmp.Compare(b.Tokens.Total(), a.Tokens.Total())
		}
		return cmp.Compare(a.Key, b.Key)
	})
	return rows
}

func groupKey(e Entry, by By, loc *time.Location) string {
	switch by {
	case BySession:
		return e.Session
	case ByDay:
		return e.Time.In(loc).Format(time.DateOnly)
	case ByConsumer:
		return string(e.Consumer)
	}
	return e.Project
}

// Sum adds rows up.
func Sum(rows []Row) Row {
	var s Row
	models := map[string]bool{}
	for _, r := range rows {
		s.Tokens = s.Tokens.Add(r.Tokens)
		s.Messages += r.Messages
		s.Cost += r.Cost
		s.Unpriced += r.Unpriced
		for _, m := range r.UnpricedModels {
			models[m] = true
		}
	}
	s.UnpricedModels = slices.Sorted(maps.Keys(models))
	return s
}
