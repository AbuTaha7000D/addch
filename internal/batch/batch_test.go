package batch

import (
	"context"
	"fmt"
	"testing"
)

func TestSequentialExecutionOrder(t *testing.T) {
	items := []string{"first", "second", "third"}
	var order []string
	action := func(ctx context.Context, item string) (ItemStatus, error, string) {
		order = append(order, item)
		return StatusSuccess, nil, ""
	}
	got := NewRunner().Run(context.Background(), items, action)

	if len(order) != 3 || order[0] != "first" || order[1] != "second" || order[2] != "third" {
		t.Errorf("execution order = %v, want %v", order, items)
	}
	if got.Succeeded != 3 || got.Skipped != 0 || got.Failed != 0 || got.Total != 3 {
		t.Errorf("summary = %+v, want all succeeded", got)
	}
}

func TestErrorContinuation(t *testing.T) {
	items := []string{"ok1", "bad", "ok2"}
	errSentinel := fmt.Errorf("boom")
	var processed []string
	action := func(ctx context.Context, item string) (ItemStatus, error, string) {
		processed = append(processed, item)
		if item == "bad" {
			return StatusFailed, errSentinel, ""
		}
		return StatusSuccess, nil, ""
	}
	got := NewRunner().Run(context.Background(), items, action)

	if len(processed) != 3 {
		t.Fatalf("a failing item must not stop later items; processed = %v", processed)
	}
	if got.Succeeded != 2 || got.Failed != 1 || got.Skipped != 0 || got.Total != 3 {
		t.Errorf("summary = %s, want Succeeded 2 / Failed 1", got.String())
	}
	bad := got.Results[1]
	if bad.Status != StatusFailed || bad.Err != errSentinel {
		t.Errorf("failed item result = %+v, want StatusFailed with the sentinel error", bad)
	}
}

func TestSkippedReporting(t *testing.T) {
	items := []string{"a", "b"}
	action := func(ctx context.Context, item string) (ItemStatus, error, string) {
		if item == "a" {
			return StatusSkipped, nil, "already processed"
		}
		return StatusSuccess, nil, ""
	}
	got := NewRunner().Run(context.Background(), items, action)

	if got.Skipped != 1 || got.Succeeded != 1 || got.Failed != 0 {
		t.Errorf("summary = %s, want 1 skipped / 1 succeeded", got.String())
	}
	skip := got.Results[0]
	if skip.Status != StatusSkipped || skip.Reason != "already processed" {
		t.Errorf("skipped result = %+v, want StatusSkipped with reason", skip)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	items := []string{"a", "b", "c"}
	var ran []string
	action := func(ctx context.Context, item string) (ItemStatus, error, string) {
		ran = append(ran, item)
		if item == "a" {
			cancel()
		}
		return StatusSuccess, nil, ""
	}
	got := NewRunner().Run(ctx, items, action)

	if len(ran) != 1 {
		t.Fatalf("expected execution to stop after cancellation, ran %d items: %v", len(ran), ran)
	}
	if got.Succeeded != 1 || got.Skipped != 2 || got.Failed != 0 || got.Total != 3 {
		t.Errorf("summary = %s, want 1 succeeded / 2 skipped", got.String())
	}
	for _, r := range got.Results[1:] {
		if r.Status != StatusSkipped || r.Reason != "cancelled" {
			t.Errorf("unprocessed item result = %+v, want StatusSkipped with cancelled reason", r)
		}
	}
}

func TestPreCancelledContextSkipsAll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	count := 0
	got := NewRunner().Run(ctx, []string{"a", "b"}, func(ctx context.Context, item string) (ItemStatus, error, string) {
		count++
		return StatusSuccess, nil, ""
	})
	if count != 0 {
		t.Errorf("no actions should run after cancellation, ran %d", count)
	}
	if got.Skipped != 2 || got.Succeeded != 0 || got.Failed != 0 {
		t.Errorf("summary = %s, want all skipped", got.String())
	}
}

func TestRunEmptyItems(t *testing.T) {
	got := NewRunner().Run(context.Background(), nil, func(ctx context.Context, item string) (ItemStatus, error, string) {
		return StatusSuccess, nil, ""
	})
	if got.Total != 0 || len(got.Results) != 0 || got.String() != "Total: 0 | Succeeded: 0 | Skipped: 0 | Failed: 0" {
		t.Errorf("empty run summary = %+v (%s)", got, got.String())
	}
}

func TestItemStatusString(t *testing.T) {
	cases := []struct {
		s    ItemStatus
		want string
	}{
		{StatusSuccess, "success"},
		{StatusSkipped, "skipped"},
		{StatusFailed, "failed"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("ItemStatus(%d).String() = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestSummaryCountingAndString(t *testing.T) {
	var s Summary
	s.Add(ItemResult{Item: "a", Status: StatusSuccess})
	s.Add(ItemResult{Item: "b", Status: StatusSkipped, Reason: "exists"})
	s.Add(ItemResult{Item: "c", Status: StatusFailed, Err: fmt.Errorf("boom")})
	s.Add(ItemResult{Item: "d", Status: StatusSuccess})

	if s.Total != 4 || s.Succeeded != 2 || s.Skipped != 1 || s.Failed != 1 {
		t.Errorf("counters = Total %d / Succeeded %d / Skipped %d / Failed %d, want 4/2/1/1",
			s.Total, s.Succeeded, s.Skipped, s.Failed)
	}
	if len(s.Results) != 4 {
		t.Errorf("Results length = %d, want 4", len(s.Results))
	}
	want := "Total: 4 | Succeeded: 2 | Skipped: 1 | Failed: 1"
	if s.String() != want {
		t.Errorf("String() = %q, want %q", s.String(), want)
	}
}
