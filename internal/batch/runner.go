package batch

import "context"

// ActionFunc processes a single batch item. It returns the item's status, an
// error (set when the item failed), and an optional skip or context reason.
type ActionFunc func(ctx context.Context, item string) (ItemStatus, error, string)

// Runner sequentially processes a list of items and aggregates their outcomes.
type Runner struct{}

// NewRunner returns a batch Runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Run executes items sequentially, one at a time. The loop continues after
// individual item failures (StatusFailed) without halting, so one bad item
// never stops the remaining items. As soon as ctx is cancelled, no further
// items are started; the unprocessed remainder is recorded as skipped with a
// "cancelled" reason so the returned Summary always covers the full item list.
func (r *Runner) Run(ctx context.Context, items []string, action ActionFunc) Summary {
	var summary Summary
	for i := 0; i < len(items); i++ {
		select {
		case <-ctx.Done():
			for _, item := range items[i:] {
				summary.Add(ItemResult{Item: item, Status: StatusSkipped, Reason: "cancelled"})
			}
			return summary
		default:
		}
		status, err, reason := action(ctx, items[i])
		summary.Add(ItemResult{Item: items[i], Status: status, Err: err, Reason: reason})
	}
	return summary
}
