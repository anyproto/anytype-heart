package client

import "time"

// StatusHook receives the client's rate-limit and retry signals — the
// producer side of the three-state model (deferred-materialization
// spec: rate limiting is NORMAL operation for a large import, and
// modelling it as an error makes every healthy run look broken). The
// sources are exact, not guessed: the shared pacer knows when and how
// long it is sleeping, the retry loop knows its bounded attempt count.
//
// Advisory telemetry only: implementations must be fast, safe for
// concurrent use (every worker's requests share one client), and must
// never affect control flow. Signals are edges, not levels — a healthy
// request stream reports nothing.
type StatusHook interface {
	// Throttled fires when a request meets the shared pushback pause
	// (a 429/529 told some worker to slow down); resumeIn is when the
	// window reopens.
	Throttled(resumeIn time.Duration)
	// Retrying fires before a backoff sleep: transient failure, attempt
	// N of the policy's M.
	Retrying(attempt, attemptsMax int)
	// Recovered fires when a request completes normally after a
	// Throttled/Retrying signal — "back to normal", never "gave up": a
	// cancelled or exhausted request does not recover.
	Recovered()
}

type noopStatusHook struct{}

func (noopStatusHook) Throttled(time.Duration) {}
func (noopStatusHook) Retrying(int, int)       {}
func (noopStatusHook) Recovered()              {}

// WithStatusHook attaches the three-state reporting seam.
func WithStatusHook(hook StatusHook) Option {
	return func(c *Client) { c.status = hook }
}
