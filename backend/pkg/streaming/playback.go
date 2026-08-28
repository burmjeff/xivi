package streaming

import (
	"context"
	"errors"
	"hash/fnv"
	"sync"
	"time"
)

const (
	sourceCapacityHandoffWait = 3 * time.Second
	playbackLeaseRetention    = 10 * time.Minute
)

type playbackLease struct {
	StreamID string
	ClientID string
	Protocol string
	LastSeen time.Time
}

// PlaybackHandle is the transport-neutral identity used inside a shared stream
// session. PlaybackID is deliberately kept out of diagnostics; ClientID is a
// fresh opaque value for each tune generation.
type PlaybackHandle struct {
	ClientID string
}

func (m *Manager) playbackStripe(id string) *sync.Mutex {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(id))
	return &m.playbackStripes[hash.Sum32()%uint32(len(m.playbackStripes))]
}

// AcquirePlaybackSources tunes a known player/session. It uses make-before-
// break when capacity is available, and break-before-make only when the old
// session can actually free a constrained source slot. This method applies to
// both HLS and MPEG-TS entry requests.
func (m *Manager) AcquirePlaybackSources(ctx context.Context, playbackID, streamID, protocol string, sources []Source) (*Session, PlaybackHandle, error) {
	return m.acquirePlaybackSources(ctx, playbackID, streamID, protocol, sources, true)
}

// StartPlaybackSources is the non-blocking counterpart to
// AcquirePlaybackSources. Capacity-aware channel handoff still completes
// before it returns, but media validation proceeds while the client is already
// attached to the session. MPEG-TS writers and cold HLS viewers both use it so
// they are visible to lifecycle and capacity management during startup.
func (m *Manager) StartPlaybackSources(ctx context.Context, playbackID, streamID, protocol string, sources []Source) (*Session, PlaybackHandle, error) {
	return m.acquirePlaybackSources(ctx, playbackID, streamID, protocol, sources, false)
}

func (m *Manager) acquirePlaybackSources(ctx context.Context, playbackID, streamID, protocol string, sources []Source, waitReady bool) (*Session, PlaybackHandle, error) {
	if playbackID == "" {
		session, err := m.acquirePlaybackSession(ctx, streamID, sources, waitReady)
		return session, PlaybackHandle{}, err
	}
	stripe := m.playbackStripe(playbackID)
	stripe.Lock()
	defer stripe.Unlock()

	m.playbackMu.Lock()
	previous, hasPrevious := m.playbacks[playbackID]
	m.playbackMu.Unlock()

	// HLS repeatedly reloads one media playlist. Those requests belong to the
	// same tune generation. MPEG-TS reconnects get a fresh generation so an old
	// response writer cannot close the replacement connection by ID.
	if hasPrevious && previous.StreamID == streamID && previous.Protocol == protocol && protocol == "hls" {
		session, err := m.acquirePlaybackSession(ctx, streamID, sources, waitReady)
		if err != nil {
			return session, PlaybackHandle{}, err
		}
		m.touchPlayback(playbackID)
		return session, PlaybackHandle{ClientID: previous.ClientID}, nil
	}

	next := playbackLease{StreamID: streamID, ClientID: randomIdentifier("con_"), Protocol: protocol, LastSeen: time.Now()}
	if !hasPrevious {
		session, err := m.acquirePlaybackSession(ctx, streamID, sources, waitReady)
		if err != nil {
			return session, PlaybackHandle{}, err
		}
		m.storePlayback(playbackID, next)
		return session, PlaybackHandle{ClientID: next.ClientID}, nil
	}

	// Reusing an existing target session consumes no additional source
	// connection. Otherwise make-before-break is safe only with free capacity.
	_, targetExists := m.Get(streamID)
	if targetExists || m.connections.hasCapacityAny(sources) {
		session, err := m.acquirePlaybackSession(ctx, streamID, sources, waitReady)
		if err == nil {
			m.storePlayback(playbackID, next)
			m.detachPlayback(ctx, previous, previous.StreamID != streamID, false, playbackEndReason(previous, next))
			return session, PlaybackHandle{ClientID: next.ClientID}, nil
		}
		if !errors.Is(err, ErrSourceConnectionLimit) {
			return session, PlaybackHandle{}, err
		}
	}

	// Do not sacrifice a working channel when it cannot free any capacity used
	// by the target. This also protects multiple viewers sharing one old stream.
	if !m.playbackCanFreeSources(previous, sources) {
		session, err := m.acquirePlaybackSession(ctx, streamID, sources, waitReady)
		if err != nil {
			return session, PlaybackHandle{}, err
		}
		m.storePlayback(playbackID, next)
		m.detachPlayback(ctx, previous, previous.StreamID != streamID, false, playbackEndReason(previous, next))
		return session, PlaybackHandle{ClientID: next.ClientID}, nil
	}

	// Publish the new generation before closing the old one. Late HLS segments
	// then fail resolution instead of resurrecting the previous session.
	m.storePlayback(playbackID, next)
	m.detachPlayback(ctx, previous, true, true, "channel_switched")
	session, err := m.acquirePlaybackSession(ctx, streamID, sources, waitReady)
	if err != nil {
		m.deletePlaybackIfCurrent(playbackID, next)
		return session, PlaybackHandle{}, err
	}
	return session, PlaybackHandle{ClientID: next.ClientID}, nil
}

func (m *Manager) acquirePlaybackSession(ctx context.Context, streamID string, sources []Source, waitReady bool) (*Session, error) {
	if waitReady {
		return m.AcquireSources(ctx, streamID, sources)
	}
	return m.StartSources(ctx, streamID, sources)
}

func (m *Manager) ResolvePlayback(playbackID, streamID string) (PlaybackHandle, bool) {
	if playbackID == "" {
		return PlaybackHandle{}, false
	}
	m.playbackMu.Lock()
	defer m.playbackMu.Unlock()
	lease, ok := m.playbacks[playbackID]
	if !ok || lease.StreamID != streamID {
		return PlaybackHandle{}, false
	}
	lease.LastSeen = time.Now()
	m.playbacks[playbackID] = lease
	return PlaybackHandle{ClientID: lease.ClientID}, true
}

// ReleasePlayback is used by controlled players on minimize, navigation, and
// page unload. Socket closure and idle cleanup remain the fallback.
func (m *Manager) ReleasePlayback(ctx context.Context, playbackID, streamID string) bool {
	if playbackID == "" {
		return false
	}
	stripe := m.playbackStripe(playbackID)
	stripe.Lock()
	defer stripe.Unlock()
	m.playbackMu.Lock()
	lease, ok := m.playbacks[playbackID]
	if !ok || (streamID != "" && lease.StreamID != streamID) {
		m.playbackMu.Unlock()
		return false
	}
	delete(m.playbacks, playbackID)
	m.playbackMu.Unlock()
	m.detachPlayback(ctx, lease, true, true, "playback_released")
	return true
}

func (m *Manager) storePlayback(id string, lease playbackLease) {
	m.playbackMu.Lock()
	m.playbacks[id] = lease
	m.playbackMu.Unlock()
}

func (m *Manager) touchPlayback(id string) {
	m.playbackMu.Lock()
	if lease, ok := m.playbacks[id]; ok {
		lease.LastSeen = time.Now()
		m.playbacks[id] = lease
	}
	m.playbackMu.Unlock()
}

func (m *Manager) deletePlaybackIfCurrent(id string, expected playbackLease) {
	m.playbackMu.Lock()
	if current, ok := m.playbacks[id]; ok && current.StreamID == expected.StreamID && current.ClientID == expected.ClientID {
		delete(m.playbacks, id)
	}
	m.playbackMu.Unlock()
}

func playbackEndReason(previous, next playbackLease) string {
	if previous.StreamID == next.StreamID {
		return "transport_replaced"
	}
	return "channel_switched"
}

func (m *Manager) detachPlayback(ctx context.Context, lease playbackLease, stopWhenEmpty, wait bool, reason string) {
	session, ok := m.Get(lease.StreamID)
	if !ok {
		return
	}
	session.ReleaseClient(lease.ClientID, reason)
	if !stopWhenEmpty || session.ActiveClientCount() != 0 {
		return
	}
	session.Stop()
	if !wait {
		return
	}
	select {
	case <-session.done:
	case <-ctx.Done():
	}
}

func (m *Manager) playbackCanFreeSources(lease playbackLease, sources []Source) bool {
	session, ok := m.Get(lease.StreamID)
	if !ok || session.ActiveClientCount() > 1 {
		return false
	}
	session.mu.RLock()
	position := session.sourceIndex
	poolID := int64(0)
	if position >= 0 && position < len(session.sources) {
		poolID = session.sources[position].PoolID
	}
	session.mu.RUnlock()
	if poolID == 0 {
		return false
	}
	for _, source := range sources {
		if source.PoolID == poolID {
			return true
		}
	}
	return false
}

// ensureSourceCapacity gives connection-oriented clients a brief handoff
// window. Plex/Jellyfin commonly request the next MPEG-TS URL just before the
// old response writer observes its closed socket. Once it closes, the now-idle
// producer is reclaimed immediately instead of lingering for IdleTimeout.
func (m *Manager) ensureSourceCapacity(ctx context.Context, sources []Source, exceptID string) {
	if m.connections.hasCapacityAny(sources) {
		return
	}
	deadline := time.Now().Add(sourceCapacityHandoffWait)
	for {
		candidates := m.idleCapacitySessions(sources, exceptID)
		for _, session := range candidates {
			session.emitEvent("info", "capacity_reclaimed", "Idle source capacity was released for a channel change.", "")
			session.Stop()
		}
		for _, session := range candidates {
			select {
			case <-session.done:
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
		if m.connections.hasCapacityAny(sources) || time.Now().After(deadline) {
			return
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (m *Manager) idleCapacitySessions(sources []Source, exceptID string) []*Session {
	pools := make(map[int64]bool)
	for _, source := range sources {
		if source.PoolID != 0 {
			pools[source.PoolID] = true
		}
	}
	m.mu.RLock()
	candidates := make([]*Session, 0)
	for id, session := range m.sessions {
		if id == exceptID || session.ActiveClientCount() != 0 {
			continue
		}
		session.mu.RLock()
		position := session.sourceIndex
		state := session.state
		poolID := int64(0)
		if position >= 0 && position < len(session.sources) {
			poolID = session.sources[position].PoolID
		}
		session.mu.RUnlock()
		if pools[poolID] && state != StateStopping && state != StateStopped {
			candidates = append(candidates, session)
		}
	}
	m.mu.RUnlock()
	return candidates
}

func (m *Manager) prunePlaybacks(now time.Time) {
	m.playbackMu.Lock()
	for id, lease := range m.playbacks {
		if now.Sub(lease.LastSeen) <= playbackLeaseRetention {
			continue
		}
		session, ok := m.Get(lease.StreamID)
		if ok && session.HasActiveClient(lease.ClientID) {
			continue
		}
		delete(m.playbacks, id)
	}
	m.playbackMu.Unlock()
}
