// Package notify shows desktop notifications (toasts), rate limited so a burst
// of drift does not flood the screen.
package notify

import (
	"sync"
	"time"

	"github.com/gen2brain/beeep"
)

type Notifier struct {
	mu    sync.Mutex
	every time.Duration
	last  time.Time
	now   func() time.Time
	send  func(title, msg string) error
}

// New returns a Notifier that shows at most one toast per interval.
func New(every time.Duration) *Notifier {
	return &Notifier{
		every: every,
		now:   time.Now,
		send:  func(title, msg string) error { return beeep.Notify(title, msg, "") },
	}
}

// Notify shows a toast unless one was shown less than the interval ago.
// sent reports whether a toast was attempted.
func (n *Notifier) Notify(title, msg string) (sent bool, err error) {
	n.mu.Lock()
	t := n.now()
	if !n.last.IsZero() && t.Sub(n.last) < n.every {
		n.mu.Unlock()
		return false, nil
	}
	n.last = t
	n.mu.Unlock()
	return true, n.send(title, msg)
}
