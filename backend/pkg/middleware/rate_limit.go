package middleware

import (
	"strconv"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/security"

	"github.com/gofiber/fiber/v2"
)

type boundedRateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]rateBucket
	maxBuckets int
}

type rateBucket struct {
	count int
	reset time.Time
}

func newBoundedRateLimiter(maxBuckets int) *boundedRateLimiter {
	return &boundedRateLimiter{buckets: make(map[string]rateBucket), maxBuckets: maxBuckets}
}

func (limiter *boundedRateLimiter) allow(key string, maximum int, window time.Duration) (bool, time.Duration) {
	now := time.Now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if len(limiter.buckets) >= limiter.maxBuckets {
		for bucketKey, bucket := range limiter.buckets {
			if now.After(bucket.reset) {
				delete(limiter.buckets, bucketKey)
			}
		}
		if len(limiter.buckets) >= limiter.maxBuckets {
			for bucketKey := range limiter.buckets {
				delete(limiter.buckets, bucketKey)
				break
			}
		}
	}
	bucket := limiter.buckets[key]
	if bucket.reset.IsZero() || now.After(bucket.reset) {
		bucket = rateBucket{reset: now.Add(window)}
	}
	bucket.count++
	limiter.buckets[key] = bucket
	return bucket.count <= maximum, time.Until(bucket.reset)
}

func BoundedRateLimit(maximum int, window time.Duration, mutationsOnly bool) fiber.Handler {
	limiter := newBoundedRateLimiter(10_000)
	return func(c *fiber.Ctx) error {
		if mutationsOnly && (c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead || c.Method() == fiber.MethodOptions) {
			return c.Next()
		}
		key := security.RequestNetworkInfo(c).IP.String()
		if principal, ok := Principal(c); ok {
			key += ":user:" + principal.Username
		}
		allowed, retry := limiter.allow(key, maximum, window)
		if allowed {
			return c.Next()
		}
		seconds := max(1, int(retry.Seconds()))
		c.Set(fiber.HeaderRetryAfter, strconv.Itoa(seconds))
		return c.Status(fiber.StatusTooManyRequests).JSON(models.APIError{Code: "rate_limited", Message: "Too many requests. Try again shortly.", Retryable: true})
	}
}

var invalidMediaAttempts = newBoundedRateLimiter(10_000)

var loginIngressAttempts = newBoundedRateLimiter(10_000)

// LoginIngressRateLimit is deliberately cheap and runs before JSON decoding,
// database access, or Argon2. Durable account/IP/network cooldowns still make
// the authorization decision; this layer only sheds obvious request floods.
func LoginIngressRateLimit() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := security.RequestNetworkInfo(c).IP.String()
		globalAllowed, globalRetry := loginIngressAttempts.allow("global", 600, time.Minute)
		addressAllowed, addressRetry := loginIngressAttempts.allow("address:"+ip, 30, time.Minute)
		if globalAllowed && addressAllowed {
			return c.Next()
		}
		retry := max(globalRetry, addressRetry)
		c.Set(fiber.HeaderRetryAfter, strconv.Itoa(max(1, int(retry.Seconds()))))
		return c.Status(fiber.StatusTooManyRequests).JSON(models.APIError{Code: "login_ingress_limited",
			Message: "Too many sign-in requests. Try again shortly.", Retryable: true})
	}
}

func rejectInvalidMediaCredential(c *fiber.Ctx) error {
	token := c.Query("access_token")
	if token != "" {
		allowed, retry := invalidMediaAttempts.allow(security.RequestNetworkInfo(c).IP.String(), 30, time.Minute)
		if !allowed {
			c.Set(fiber.HeaderRetryAfter, strconv.Itoa(max(1, int(retry.Seconds()))))
			return securityError(c, fiber.StatusTooManyRequests, "media_rate_limited", "Too many invalid media credentials.")
		}
	}
	return securityError(c, fiber.StatusUnauthorized, "media_authentication_required", "A valid media key is required.")
}
