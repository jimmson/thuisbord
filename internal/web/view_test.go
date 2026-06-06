package web

import (
	"testing"
	"time"

	"thuisbord/internal/ovapi"
)

// helper: build a one-stop board from (line, status, minutesFromNow) tuples.
func boardAt(now time.Time, deps ...struct {
	line, status string
	min          int
}) ovapi.Board {
	stop := ovapi.Stop{Name: "Test Stop"}
	for _, d := range deps {
		t := now.Add(time.Duration(d.min) * time.Minute)
		stop.Departures = append(stop.Departures, ovapi.Departure{
			Line:        d.line,
			Destination: "Somewhere",
			Target:      t,
			Expected:    t,
			Status:      d.status,
		})
	}
	return ovapi.Board{Stops: []ovapi.Stop{stop}}
}

func TestBuildBusView_CapReservedForRealDepartures(t *testing.T) {
	now := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	// Two cancelled trips sort soonest, then real ones. With max=3 we must
	// still see all 3 soonest REAL departures, not have them crowded out.
	board := boardAt(now,
		struct {
			line, status string
			min          int
		}{"801", "CANCEL", 1},
		struct {
			line, status string
			min          int
		}{"100", "DRIVING", 2},
		struct {
			line, status string
			min          int
		}{"305", "DRIVING", 3},
		struct {
			line, status string
			min          int
		}{"370", "PLANNED", 4},
		struct {
			line, status string
			min          int
		}{"103", "PLANNED", 5},
	)
	v := buildBusView(board, time.UTC, 8*time.Minute, 3, now, now, true)

	real := 0
	for _, d := range v.Departures {
		if !d.Cancelled {
			real++
		}
	}
	if real != 3 {
		t.Fatalf("expected 3 real departures kept, got %d (%+v)", real, v.Departures)
	}
	if !hasLine(v.Departures, "305") {
		t.Errorf("305 should be present, was crowded out by a cancellation")
	}
}

func TestBuildBusView_CancellationsBounded(t *testing.T) {
	now := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	board := boardAt(now,
		mk("801", "CANCEL", 1), mk("802", "CANCEL", 2), mk("803", "CANCEL", 3),
		mk("804", "CANCEL", 4), mk("100", "DRIVING", 5),
	)
	v := buildBusView(board, time.UTC, 8*time.Minute, 6, now, now, true)

	cancelled := 0
	for _, d := range v.Departures {
		if d.Cancelled {
			cancelled++
		}
	}
	if cancelled > 2 {
		t.Errorf("expected at most 2 cancelled rows shown, got %d", cancelled)
	}
}

func TestBuildBusView_AllCancelledNotEmpty(t *testing.T) {
	now := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	board := boardAt(now, mk("801", "CANCEL", 2), mk("802", "CANCEL", 5))
	v := buildBusView(board, time.UTC, 8*time.Minute, 6, now, now, true)

	if v.Empty {
		t.Errorf("a board with cancelled trips should not be reported Empty")
	}
	if len(v.Departures) == 0 {
		t.Errorf("cancelled trips should be shown as context, got none")
	}
}

func TestBuildBusView_GraceWindow(t *testing.T) {
	now := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	board := boardAt(now,
		mk("A", "DRIVING", -1), // 1 min ago — kept (grace)
		mk("B", "DRIVING", -3), // 3 min ago — dropped
		mk("C", "DRIVING", 5),
	)
	v := buildBusView(board, time.UTC, 8*time.Minute, 6, now, now, true)

	if !hasLine(v.Departures, "A") {
		t.Errorf("a just-departed bus (1 min ago) should linger within the grace window")
	}
	if hasLine(v.Departures, "B") {
		t.Errorf("a bus 3 min past should be dropped")
	}
}

func mk(line, status string, min int) struct {
	line, status string
	min          int
} {
	return struct {
		line, status string
		min          int
	}{line, status, min}
}

func hasLine(deps []BusDeparture, line string) bool {
	for _, d := range deps {
		if d.Line == line {
			return true
		}
	}
	return false
}
