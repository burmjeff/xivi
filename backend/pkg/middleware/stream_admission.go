package middleware

import (
	"fmt"
	"strconv"
	"sync"
	"time"
	"xivi/backend/pkg/security"
	"xivi/backend/pkg/streaming"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

type StreamAdmissionDecision struct {
	Code       string
	Message    string
	RetryAfter time.Duration
}

type StreamAdmissionLease struct {
	once sync.Once
	keys []string
}

var streamStartBudgets = newBoundedTokenLimiter(20_000)

var streamReservations = struct {
	sync.Mutex
	items map[string][]time.Time
}{items: make(map[string][]time.Time)}

func streamAdmissionIdentity(c *fiber.Ctx) (kind string, id, ownerID int64) {
	if principal, ok := Principal(c); ok {
		ownerID = principal.UserID
		if session, sessionOK := CurrentMobileSession(c); sessionOK {
			return "mobile_session", session.ID, ownerID
		}
		if session, sessionOK := CurrentSession(c); sessionOK {
			return "session", session.ID, ownerID
		}
		return "user", principal.UserID, ownerID
	}
	if credential, ok := CurrentMediaCredential(c); ok {
		if credential.Key != nil {
			return "media_key", credential.Key.ID, credential.Key.UserID
		}
		return "virtual_tuner", credential.VirtualLineupID, 0
	}
	return "", 0, 0
}

func pruneStreamReservations(now time.Time) {
	for key, expiries := range streamReservations.items {
		kept := expiries[:0]
		for _, expiry := range expiries {
			if now.Before(expiry) {
				kept = append(kept, expiry)
			}
		}
		if len(kept) == 0 {
			delete(streamReservations.items, key)
		} else {
			streamReservations.items[key] = kept
		}
	}
}

func (lease *StreamAdmissionLease) Release() {
	if lease == nil {
		return
	}
	lease.once.Do(func() {
		streamReservations.Lock()
		for _, key := range lease.keys {
			items := streamReservations.items[key]
			if len(items) <= 1 {
				delete(streamReservations.items, key)
			} else {
				streamReservations.items[key] = items[1:]
			}
		}
		streamReservations.Unlock()
	})
}

// AdmitStreamStart protects only a genuinely new downstream connection.
// Existing HLS viewers and every playlist/segment refresh bypass this gate.
func AdmitStreamStart(c *fiber.Ctx, existing bool) (*StreamAdmissionLease, *StreamAdmissionDecision) {
	if existing {
		return &StreamAdmissionLease{}, nil
	}
	kind, id, ownerID := streamAdmissionIdentity(c)
	if kind == "" || id < 1 {
		return nil, &StreamAdmissionDecision{Code: "stream_authentication_required", Message: "A valid playback identity is required.", RetryAfter: time.Minute}
	}
	configured := settings.Current()
	userLimit := max(1, configured.Streaming.MaxConcurrentStreamsPerUser)
	credentialLimit := 0
	if kind == "media_key" {
		credentialLimit = max(1, configured.Streaming.MaxConcurrentStreamsPerKey)
	} else if kind == "virtual_tuner" {
		credentialLimit = max(1, configured.VirtualTuner.TunerCount)
	}
	credentialActive, ownerActive := streaming.DefaultManager.ActiveViewerCounts(kind, id, ownerID)
	credentialKey := fmt.Sprintf("credential:%s:%d", kind, id)
	ownerKey := fmt.Sprintf("owner:%d", ownerID)
	now := time.Now()
	streamReservations.Lock()
	pruneStreamReservations(now)
	if ownerID > 0 && ownerActive+len(streamReservations.items[ownerKey]) >= userLimit {
		activeOrStarting := ownerActive + len(streamReservations.items[ownerKey])
		streamReservations.Unlock()
		RecordAutomationEvent(c, "stream_concurrency_exceeded", "account", "active_or_starting="+strconv.Itoa(activeOrStarting))
		return nil, &StreamAdmissionDecision{Code: "stream_concurrency_exceeded", Message: "This account is already using its available streams.", RetryAfter: 5 * time.Second}
	}
	if credentialLimit > 0 && credentialActive+len(streamReservations.items[credentialKey]) >= credentialLimit {
		activeOrStarting := credentialActive + len(streamReservations.items[credentialKey])
		streamReservations.Unlock()
		RecordAutomationEvent(c, "stream_concurrency_exceeded", kind, "active_or_starting="+strconv.Itoa(activeOrStarting))
		return nil, &StreamAdmissionDecision{Code: "stream_concurrency_exceeded", Message: "This device is already using its available streams.", RetryAfter: 5 * time.Second}
	}
	starts := max(1, configured.Streaming.StreamStartsPerMinute)
	policies := []tokenPolicy{{key: "stream-address:" + security.RequestNetworkInfo(c).IP.String(), capacity: float64(starts * 4), perSecond: float64(starts) / 15}}
	if ownerID > 0 {
		policies = append(policies, tokenPolicy{key: "stream-" + ownerKey, capacity: float64(starts), perSecond: float64(starts) / 60})
	}
	if kind == "media_key" || kind == "virtual_tuner" {
		policies = append(policies, tokenPolicy{key: "stream-" + credentialKey, capacity: float64(starts), perSecond: float64(starts) / 60})
	}
	allowed, retry, _ := streamStartBudgets.consume(policies, 1)
	if !allowed {
		streamReservations.Unlock()
		RecordAutomationEvent(c, "stream_start_rate_exceeded", kind, "new_stream_start_budget_exhausted")
		return nil, &StreamAdmissionDecision{Code: "stream_start_rate_exceeded", Message: "Too many streams were started recently. Try again shortly.", RetryAfter: retry}
	}
	keys := []string{}
	if ownerID > 0 {
		streamReservations.items[ownerKey] = append(streamReservations.items[ownerKey], now.Add(45*time.Second))
		keys = append(keys, ownerKey)
	}
	if credentialLimit > 0 {
		streamReservations.items[credentialKey] = append(streamReservations.items[credentialKey], now.Add(45*time.Second))
		keys = append(keys, credentialKey)
	}
	streamReservations.Unlock()
	return &StreamAdmissionLease{keys: keys}, nil
}
