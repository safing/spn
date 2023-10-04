package terminal

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/safing/portbase/log"
)

const (
	rateLimitMinOps          = 100
	rateLimitMaxOpsPerSecond = 20 // TODO: Reduce to 10 after test phase.

	rateLimitMinSuspicion          = 25
	rateLimitMaxSuspicionPerSecond = 2 // TODO: Reduce to 1 after test phase.
)

// Session holds terminal metadata for operations.
type Session struct {
	sync.RWMutex

	// Rate Limiting.

	// started holds the unix timestamp in seconds when the session was started.
	// It is set when the Session is created and may be treated as a constant.
	started int64

	// opCount is the amount of operations started.
	opCount atomic.Int64

	// suspicionScore holds a score of suspicious activity.
	// Every suspicious operations is counted as at least 1.
	// Rate limited operations because of suspicion are also counted as 1.
	suspicionScore atomic.Int64
}

// SessionTerminal is an interface for terminals that support authorization.
type SessionTerminal interface {
	GetSession() *Session
}

// SessionAddOn can be inherited by terminals to add support for sessions.
type SessionAddOn struct {
	lock sync.Mutex

	// session holds the terminal session.
	session *Session
}

// GetSession returns the terminal's session.
func (t *SessionAddOn) GetSession() *Session {
	t.lock.Lock()
	defer t.lock.Unlock()

	// Create session if it does not exist.
	if t.session == nil {
		t.session = &Session{
			started: time.Now().Unix() - 1, // Ensure a 1 second difference to current time.
		}
	}

	return t.session
}

// RateLimitInfo returns some basic information about the status of the rate limiter.
func (s *Session) RateLimitInfo() string {
	secondsActive := time.Now().Unix() - s.started

	return fmt.Sprintf(
		"%do/s %ds/s %ds",
		s.opCount.Load()/secondsActive,
		s.suspicionScore.Load()/secondsActive,
		secondsActive,
	)
}

// RateLimit enforces a rate and suspicion limit.
func (s *Session) RateLimit() *Error {
	secondsActive := time.Now().Unix() - s.started

	// Check the suspicion limit.
	score := s.suspicionScore.Load()
	if score >= rateLimitMinSuspicion {
		scorePerSecond := score / secondsActive
		if scorePerSecond >= rateLimitMaxSuspicionPerSecond {
			// Add current try to suspicion score.
			s.suspicionScore.Add(1)

			return ErrRateLimited
		}
	}

	// Check the rate limit.
	count := s.opCount.Add(1)
	if count >= rateLimitMinOps {
		opsPerSecond := count / secondsActive
		if opsPerSecond >= rateLimitMaxOpsPerSecond {
			return ErrRateLimited
		}
	}

	return nil
}

// Suspicion Factors.
const (
	SusFactorCommon          = 1
	SusFactorWeirdButOK      = 5
	SusFactorQuiteUnusual    = 10
	SusFactorMustBeMalicious = 100
)

// ReportSuspiciousActivity reports suspicious activity of the terminal.
func (s *Session) ReportSuspiciousActivity(factor int64) {
	log.Debugf("session: suspicion raised by %d", factor)
	s.suspicionScore.Add(factor)
}
