package middleware

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/security"
	"xivi/backend/platform/database"

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

type tokenBucket struct {
	tokens   float64
	updated  time.Time
	lastSeen time.Time
}

type tokenPolicy struct {
	key       string
	capacity  float64
	perSecond float64
}

type boundedTokenLimiter struct {
	mu         sync.Mutex
	buckets    map[string]tokenBucket
	maxBuckets int
}

func newBoundedTokenLimiter(maxBuckets int) *boundedTokenLimiter {
	return &boundedTokenLimiter{buckets: make(map[string]tokenBucket), maxBuckets: maxBuckets}
}

func (limiter *boundedTokenLimiter) consume(policies []tokenPolicy, cost float64) (bool, time.Duration, string) {
	now := time.Now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if len(limiter.buckets) >= limiter.maxBuckets {
		for key, bucket := range limiter.buckets {
			if now.Sub(bucket.lastSeen) > 10*time.Minute {
				delete(limiter.buckets, key)
			}
		}
	}
	if len(limiter.buckets) >= limiter.maxBuckets {
		oldestKey := ""
		var oldest time.Time
		for key, bucket := range limiter.buckets {
			if oldestKey == "" || bucket.lastSeen.Before(oldest) {
				oldestKey, oldest = key, bucket.lastSeen
			}
		}
		delete(limiter.buckets, oldestKey)
	}

	retry := time.Duration(0)
	blocked := ""
	updated := make(map[string]tokenBucket, len(policies))
	for _, policy := range policies {
		bucket, exists := limiter.buckets[policy.key]
		if !exists {
			bucket = tokenBucket{tokens: policy.capacity, updated: now}
		} else {
			bucket.tokens = min(policy.capacity, bucket.tokens+now.Sub(bucket.updated).Seconds()*policy.perSecond)
			bucket.updated = now
		}
		bucket.lastSeen = now
		updated[policy.key] = bucket
		if bucket.tokens < cost {
			wait := time.Duration((cost - bucket.tokens) / policy.perSecond * float64(time.Second))
			if wait > retry {
				retry = wait
				blocked = policy.key
			}
		}
	}
	for key, bucket := range updated {
		limiter.buckets[key] = bucket
	}
	if blocked != "" {
		return false, max(time.Second, retry), blocked
	}
	for _, policy := range policies {
		bucket := limiter.buckets[policy.key]
		bucket.tokens -= cost
		limiter.buckets[policy.key] = bucket
	}
	return true, 0, ""
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

var requestBudgets = newBoundedTokenLimiter(25_000)

var automationSignals = newBoundedRateLimiter(20_000)

var anomalyAuditSignals = newBoundedRateLimiter(20_000)

func requestNetworkPrefix(ip net.IP) string {
	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4.Mask(net.CIDRMask(24, 32)).String() + "/24"
	}
	if ipv6 := ip.To16(); ipv6 != nil {
		return ipv6.Mask(net.CIDRMask(56, 128)).String() + "/56"
	}
	return "unknown"
}

func requestBudgetPolicies(c *fiber.Ctx) []tokenPolicy {
	network := security.RequestNetworkInfo(c)
	policies := []tokenPolicy{
		{key: "network:" + requestNetworkPrefix(network.IP), capacity: 4800, perSecond: 80},
		{key: "address:" + network.IP.String(), capacity: 1200, perSecond: 20},
	}
	if principal, ok := Principal(c); ok {
		policies = append(policies, tokenPolicy{key: fmt.Sprintf("user:%d", principal.UserID), capacity: 600, perSecond: 10})
		if session, sessionOK := CurrentSession(c); sessionOK {
			policies = append(policies, tokenPolicy{key: fmt.Sprintf("session:%d", session.ID), capacity: 480, perSecond: 8})
		}
	}
	if credential, ok := CurrentMediaCredential(c); ok {
		if credential.Key != nil {
			policies = append(policies, tokenPolicy{key: fmt.Sprintf("media-key:%d", credential.Key.ID), capacity: 300, perSecond: 5})
		} else if credential.VirtualLineupID > 0 {
			policies = append(policies, tokenPolicy{key: fmt.Sprintf("virtual-tuner:%d", credential.VirtualLineupID), capacity: 600, perSecond: 10})
		}
	}
	return policies
}

func requestCost(c *fiber.Ctx, base int) float64 {
	cost := max(1, base)
	if value, err := strconv.Atoi(c.Query("limit")); err == nil && value > 100 {
		cost += (min(value, 500) - 1) / 100
	}
	if c.Query("cursor") != "" {
		cost++
	}
	if strings.Contains(c.Path(), "/guide") {
		from, fromErr := time.Parse(time.RFC3339, c.Query("from"))
		to, toErr := time.Parse(time.RFC3339, c.Query("to"))
		if fromErr == nil && toErr == nil && to.After(from) {
			cost += min(6, int(to.Sub(from)/(12*time.Hour)))
		}
	}
	return float64(cost)
}

// RequestBudget applies independent token buckets to the network, address,
// account/session, and media credential. Large pages, guide windows, and deep
// pagination cost more than ordinary navigation.
func RequestBudget(baseCost int, category string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		allowed, retry, dimension := requestBudgets.consume(requestBudgetPolicies(c), requestCost(c, baseCost))
		if allowed {
			return c.Next()
		}
		RecordAutomationEvent(c, "request_budget_exceeded", category, "blocked_dimension="+strings.SplitN(dimension, ":", 2)[0])
		c.Set(fiber.HeaderRetryAfter, strconv.Itoa(max(1, int(retry.Seconds()))))
		return c.Status(fiber.StatusTooManyRequests).JSON(models.APIError{Code: "request_budget_exceeded",
			Message: "Too many requests. Try again shortly.", Retryable: true})
	}
}

func automationIdentity(c *fiber.Ctx) string {
	if principal, ok := Principal(c); ok {
		return fmt.Sprintf("user:%d", principal.UserID)
	}
	if credential, ok := CurrentMediaCredential(c); ok {
		if credential.Key != nil {
			return fmt.Sprintf("media-key:%d", credential.Key.ID)
		}
		return fmt.Sprintf("virtual-tuner:%d", credential.VirtualLineupID)
	}
	return "address:" + security.RequestNetworkInfo(c).IP.String()
}

// RecordAutomationEvent writes at most one event of each kind per identity
// every five minutes. Security telemetry therefore remains useful without an
// attacker being able to turn the audit log itself into a storage attack.
func RecordAutomationEvent(c *fiber.Ctx, action, resourceID, detail string) {
	recordAutomationEvent(c, action, "blocked", resourceID, detail)
}

func RecordAutomationObservation(c *fiber.Ctx, action, resourceID, detail string) {
	recordAutomationEvent(c, action, "observed", resourceID, detail)
}

func recordAutomationEvent(c *fiber.Ctx, action, outcome, resourceID, detail string) {
	identity := automationIdentity(c)
	if allowed, _ := anomalyAuditSignals.allow(action+":"+identity, 1, 5*time.Minute); !allowed || database.Db == nil {
		return
	}
	event := models.SecurityAuditEvent{Action: action, Outcome: outcome, ResourceType: "automation_guard",
		ResourceID: resourceID, ClientIP: security.RequestNetworkInfo(c).IP.String(), Detail: detail}
	if principal, ok := Principal(c); ok {
		event.ActorUserID = &principal.UserID
		event.ActorUsername = principal.Username
		event.ActorDisplayName = principal.DisplayName
	} else if credential, ok := CurrentMediaCredential(c); ok && credential.Key != nil {
		event.ActorUserID = &credential.Key.UserID
		event.ActorUsername = credential.Key.Username
	}
	_ = database.Db.Audit(context.Background(), event)
}

// ObserveAutomationSignals records repeated authorization and enumeration
// misses on sensitive routes. It deliberately does not block a single miss.
func ObserveAutomationSignals(c *fiber.Ctx) error {
	err := c.Next()
	path := c.Path()
	if (c.Response().StatusCode() == fiber.StatusForbidden || c.Response().StatusCode() == fiber.StatusNotFound) &&
		(strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/media/") || strings.HasPrefix(path, "/stream/") || strings.HasPrefix(path, "/images/")) {
		identity := automationIdentity(c)
		if allowed, _ := automationSignals.allow("miss:"+identity, 40, time.Minute); !allowed {
			RecordAutomationEvent(c, "resource_enumeration_detected", "protected_route", "repeated_forbidden_or_missing_responses")
		}
	}
	return err
}

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
