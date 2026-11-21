package prepush

// Patterns lists the employer material that must never reach this public repo.
//
// They are assembled from fragments rather than written out: a scanner whose
// own source spells the words it forbids matches itself, including in the
// history scan. Matching is case-insensitive.
func Patterns() []string {
	h := "haga"
	org := h + "link"
	return []string{
		org,       // the company, its domain, its GitHub org
		h + "box", // products
		h + "nonimizer",
		"blak" + "bill",
		"oku" + "pas",
		"Hu Fash" + "ion Tech", // a named client on the report cover
		"jose" + "ma",          // a colleague, in a lead-source multiplier
		"ph-igna" + "cio|ph-fr" + "an|ph-al" + "ex", // colleague portrait slots
		"super" + `admin@test\.com`,                 // demo credentials against production
		// Bounded: an unanchored match also hits fixtures like "TestPassword123!",
		// which are generic sample data in third-party skills.
		`\bpass` + `word123\b`,
		"Patios de C" + "órdoba",                                 // a real campaign, used as live test data
		"Partner (Esen" + "cial|Avan" + "zado|Priori" + "tario)", // the retainer plans
		"back" + "end 35 . front" + "end 30",                     // internal hourly cost by role
		`(290|590|1\.150) €/mes`,                                 // retainer prices
	}
}
