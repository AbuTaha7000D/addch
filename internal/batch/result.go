// Package batch provides a generic, sequential orchestration engine for the
// addch toolkit's batch modes (Phase 4 and later). It deliberately has no
// knowledge of chapters, files, or media; directory scanning and candidate
// discovery are out of scope here.
package batch

import "fmt"

// ItemStatus is the outcome of processing a single batch item.
type ItemStatus int

const (
	// StatusSuccess indicates the item was processed successfully.
	StatusSuccess ItemStatus = iota
	// StatusSkipped indicates the item was intentionally not processed.
	StatusSkipped
	// StatusFailed indicates processing returned an error.
	StatusFailed
)

// String renders a human-readable status name.
func (s ItemStatus) String() string {
	switch s {
	case StatusSuccess:
		return "success"
	case StatusSkipped:
		return "skipped"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// ItemResult records the outcome of one item in a batch run.
type ItemResult struct {
	// Item identifies the item (by path or other identifier).
	Item string
	// Status is the item's outcome.
	Status ItemStatus
	// Err is set when Status == StatusFailed, nil otherwise.
	Err error
	// Reason explains a skip or carries extra context.
	Reason string
}

// Summary aggregates the outcome of a batch run.
type Summary struct {
	Total     int
	Succeeded int
	Skipped   int
	Failed    int
	Results   []ItemResult
}

// String renders a standardized one-line summary.
func (s Summary) String() string {
	return fmt.Sprintf("Total: %d | Succeeded: %d | Skipped: %d | Failed: %d",
		s.Total, s.Succeeded, s.Skipped, s.Failed)
}

// Add appends a result and increments the corresponding counters.
func (s *Summary) Add(res ItemResult) {
	s.Total++
	s.Results = append(s.Results, res)
	switch res.Status {
	case StatusSuccess:
		s.Succeeded++
	case StatusSkipped:
		s.Skipped++
	case StatusFailed:
		s.Failed++
	}
}
