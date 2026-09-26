package projects

// Board order: every issue has a float sort_order; smaller is higher in its
// column. Moving an issue puts it halfway between its new neighbours, so a
// drag only rewrites one row. When two neighbours get too close the column is
// renumbered (rebalance). The frontend mirrors this in sortOrder.ts for its
// optimistic update.

const (
	sortGap = 1024.0
	minGap  = 1e-6
)

// between returns a sort order for a slot after prev and before next (nil
// means no neighbour on that side). ok is false when the neighbours are too
// close and the column needs a rebalance first.
func between(prev, next *float64) (float64, bool) {
	switch {
	case prev != nil && next != nil:
		if *next-*prev <= minGap {
			return 0, false
		}
		return (*prev + *next) / 2, true
	case prev != nil:
		return *prev + sortGap, true
	case next != nil:
		return *next - sortGap, true
	default:
		return 0, true
	}
}

// spread returns evenly spaced sort orders for n issues, used by rebalance.
func spread(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = float64(i+1) * sortGap
	}
	return out
}
