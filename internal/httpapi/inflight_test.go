package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"


)

func TestInflightTrackerWaitsForEveryTaskAndHonoursDeadline(t *testing.T) {
	tracker := newInflightTracker()
	if err := tracker.Wait(context.Background()); err != nil {
		t.Fatal("idle tracker must not block", err)
	}
	first, second := tracker.Add(), tracker.Add()
	if tracker.Count() != 2 {
		t.Fatal(tracker.Count())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := tracker.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("wait returned before tasks finished", err)
	}
	first()
	first() // completion is idempotent
	if tracker.Count() != 1 {
		t.Fatal("double completion miscounted", tracker.Count())
	}
	waited := make(chan error, 1)
	go func() { waited <- tracker.Wait(context.Background()) }()
	select {
	case <-waited:
		t.Fatal("wait returned with a task still running")
	case <-time.After(30 * time.Millisecond):
	}
	second()
	select {
	case err := <-waited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not observe the last completion")
	}
}

func TestBackgroundControlsAreNilSafeAndRecordHandover(t *testing.T) {
	bg := &Background{}
	bg.StopClaiming()
	bg.StopClaiming()
	bg.Stop()
	var nilBG *Background
	nilBG.StopClaiming()
	nilBG.HandedOver()
	nilBG.Stop()
	if nilBG.handedOverToSuccessor() {
		t.Fatal("nil background reported a handover")
	}
	bg.HandedOver()
	if !bg.handedOverToSuccessor() {
		t.Fatal("handover not recorded")
	}
}

func TestRunAfterGateDefersUntilClosed(t *testing.T) {
	ran := make(chan struct{}, 2)
	runAfterGate(nil, func() { ran <- struct{}{} })
	select {
	case <-ran:
	default:
		t.Fatal("nil gate must run synchronously")
	}
	gate := make(chan struct{})
	runAfterGate(gate, func() { ran <- struct{}{} })
	select {
	case <-ran:
		t.Fatal("ran before gate closed")
	case <-time.After(30 * time.Millisecond):
	}
	close(gate)
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("did not run after gate closed")
	}
}

