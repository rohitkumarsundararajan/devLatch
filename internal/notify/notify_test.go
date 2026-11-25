package notify

import (
	"testing"
	"time"
)

func TestRateLimit(t *testing.T) {
	calls := 0
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	n := &Notifier{every: 10 * time.Second, now: func() time.Time { return now },
		send: func(string, string) error { calls++; return nil }}

	if sent, _ := n.Notify("a", "1"); !sent {
		t.Fatal("first toast should be sent")
	}
	now = now.Add(5 * time.Second)
	if sent, _ := n.Notify("a", "2"); sent {
		t.Fatal("second toast inside the interval should be skipped")
	}
	now = now.Add(6 * time.Second)
	if sent, _ := n.Notify("a", "3"); !sent {
		t.Fatal("toast after the interval should be sent")
	}
	if calls != 2 {
		t.Fatalf("send called %d times, want 2", calls)
	}
}
