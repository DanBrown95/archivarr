package db_test

import (
	"context"
	"testing"
	"time"
)

func TestAutomationPauseResume(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Default: not paused.
	if paused, _, _ := d.AutomationPaused(ctx); paused {
		t.Fatal("should not be paused by default")
	}

	// Indefinite pause.
	if err := d.PauseAutomation(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if paused, until, _ := d.AutomationPaused(ctx); !paused || until != nil {
		t.Fatalf("expected indefinite pause, got paused=%v until=%v", paused, until)
	}

	// Resume.
	if err := d.ResumeAutomation(ctx); err != nil {
		t.Fatal(err)
	}
	if paused, _, _ := d.AutomationPaused(ctx); paused {
		t.Fatal("should be resumed")
	}
}

func TestAutomationTimedPauseExpires(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Already-elapsed expiry → treated as not paused (and auto-resumed).
	past := time.Now().Unix() - 1
	if err := d.PauseAutomation(ctx, &past); err != nil {
		t.Fatal(err)
	}
	if paused, _, _ := d.AutomationPaused(ctx); paused {
		t.Fatal("expired timed pause should report not paused")
	}

	// Future expiry → paused.
	future := time.Now().Unix() + 3600
	if err := d.PauseAutomation(ctx, &future); err != nil {
		t.Fatal(err)
	}
	paused, until, _ := d.AutomationPaused(ctx)
	if !paused || until == nil || *until != future {
		t.Fatalf("expected paused until %d, got paused=%v until=%v", future, paused, until)
	}
}
