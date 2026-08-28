package middleware

import (
	"testing"
	"time"
)

func TestTokenBudgetConsumesEveryIndependentDimension(t *testing.T) {
	limiter := newBoundedTokenLimiter(10)
	policies := []tokenPolicy{
		{key: "address:one", capacity: 3, perSecond: 1.0 / 60},
		{key: "user:one", capacity: 2, perSecond: 1.0 / 60},
	}
	for attempt := 0; attempt < 2; attempt++ {
		allowed, _, _ := limiter.consume(policies, 1)
		if !allowed {
			t.Fatalf("attempt %d was unexpectedly denied", attempt)
		}
	}
	allowed, retry, blocked := limiter.consume(policies, 1)
	if allowed || blocked != "user:one" || retry < time.Second {
		t.Fatalf("user dimension did not independently block: allowed=%v retry=%s blocked=%q", allowed, retry, blocked)
	}
}
