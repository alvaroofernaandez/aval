package relay

// SelectTail returns the handles of the latest messages to keep verbatim,
// oldest first. It cuts only where every tool result kept has its tool call
// kept too; a call still waiting for its result (mid-turn) may be kept. The
// longest tail within budget wins; when even the shortest valid tail is over
// budget, that tail is kept alone.
func SelectTail(msgs []Message, budget int) []string {
	pending := map[string]bool{} // results whose call is not in the tail yet
	sum, best, shortest := 0, -1, -1
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		sum += m.Tokens
		for _, id := range m.ToolResults {
			pending[id] = true
		}
		for _, id := range m.ToolUses {
			delete(pending, id)
		}
		if len(pending) > 0 {
			continue
		}
		if shortest < 0 {
			shortest = i
		}
		if sum > budget {
			break
		}
		best = i
	}
	start := best
	if start < 0 {
		start = shortest
	}
	if start < 0 {
		return nil
	}
	handles := make([]string, 0, len(msgs)-start)
	for _, m := range msgs[start:] {
		handles = append(handles, m.Handle)
	}
	return handles
}
