package twelvedata

import (
	"context"
	"testing"
	"time"
)

func TestLimiterReservesSlotsForInteractive(t *testing.T) {
	l := NewLimiter(4, 2, time.Hour)

	for i := 0; i < 2; i++ { // background may use max-reserve = 2
		if _, ok := l.tryTake(false); !ok {
			t.Fatalf("background slot %d should be free", i)
		}
	}
	if _, ok := l.tryTake(false); ok {
		t.Fatal("background must not touch the reserved slots")
	}
	for i := 0; i < 2; i++ { // interactive can use the rest
		if _, ok := l.tryTake(true); !ok {
			t.Fatalf("interactive slot %d should be free", i)
		}
	}
	if _, ok := l.tryTake(true); ok {
		t.Fatal("window is full")
	}
}

func TestLimiterWindowSlides(t *testing.T) {
	l := NewLimiter(1, 0, 40*time.Millisecond)
	if err := l.Wait(context.Background(), true); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if err := l.Wait(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 30*time.Millisecond {
		t.Fatal("second call should have waited for the window to slide")
	}
}

func TestLimiterBackgroundYieldsToWaitingUser(t *testing.T) {
	l := NewLimiter(1, 0, time.Hour)
	_ = l.Wait(context.Background(), true) // fill the window

	l.mu.Lock()
	l.waiting = 1
	l.mu.Unlock()
	if _, ok := l.tryTake(false); ok {
		t.Fatal("background must yield while a user waits")
	}
}

func TestLimiterWaitHonoursContext(t *testing.T) {
	l := NewLimiter(1, 0, time.Hour)
	_ = l.Wait(context.Background(), true)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := l.Wait(ctx, true); err == nil {
		t.Fatal("expected context error")
	}
}
