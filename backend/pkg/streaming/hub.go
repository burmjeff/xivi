package streaming

import (
	"sync"
	"sync/atomic"
)

const canonicalTransportChunkBytes = 7 * 188

type mediaChunk struct {
	data         []byte
	hasPAT       bool
	hasPMT       bool
	randomAccess bool
}

// Hub fans one producer out to many MPEG-TS viewers. A slow viewer owns only
// its bounded queue; it can never block the GStreamer streaming thread or the
// other viewers.
type Hub struct {
	mu              sync.Mutex
	subscribers     map[uint64]*subscriber
	nextID          uint64
	ring            []mediaChunk
	ringBytes       int
	maxRingBytes    int
	maxClientBytes  int
	transportSeen   bool
	programmeKnown  bool
	hasVideo        bool
	videoPIDs       map[uint16]bool
	closed          bool
	closeErr        error
	bytesPublished  atomic.Uint64
	slowClientDrops atomic.Uint64
}

type subscriber struct {
	id          uint64
	chunks      chan mediaChunk
	pending     int
	closed      bool
	closeReason string
	waiting     bool
}

// Subscription is a single bounded view of a shared MPEG-TS stream.
type Subscription struct {
	hub  *Hub
	sub  *subscriber
	once sync.Once
}

func NewHub(maxClientBytes int) *Hub {
	if maxClientBytes < 1024*1024 {
		maxClientBytes = 1024 * 1024
	}
	return &Hub{
		subscribers:    make(map[uint64]*subscriber),
		maxRingBytes:   maxClientBytes / 2,
		maxClientBytes: maxClientBytes,
	}
}

func (h *Hub) Publish(data []byte) {
	h.publish(data, false)
}

// PublishOwned transfers an immutable chunk already owned by the streaming
// layer. It avoids another full transport-stream copy in the session bridge.
func (h *Hub) PublishOwned(data []byte) {
	h.publish(data, true)
}

func (h *Hub) publish(data []byte, ownedData bool) {
	if len(data) == 0 {
		return
	}
	owned := data
	if !ownedData {
		owned = append([]byte(nil), data...)
	}
	metadata := inspectTransportChunk(owned)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	if metadata.isTransport {
		h.transportSeen = true
	}
	if metadata.programmeKnown {
		h.programmeKnown = true
		h.hasVideo = h.hasVideo || metadata.hasVideo
		if len(metadata.videoPIDs) > 0 {
			if h.videoPIDs == nil {
				h.videoPIDs = make(map[uint16]bool)
			}
			for _, pid := range metadata.videoPIDs {
				h.videoPIDs[pid] = true
			}
		}
	}
	randomVideoAccess := false
	for _, pid := range metadata.randomAccessPIDs {
		if h.videoPIDs[pid] {
			randomVideoAccess = true
			break
		}
	}
	chunk := mediaChunk{
		data:         owned,
		hasPAT:       metadata.hasPAT,
		hasPMT:       metadata.hasPMT,
		randomAccess: randomVideoAccess,
	}

	h.bytesPublished.Add(uint64(len(owned)))
	h.ring = append(h.ring, chunk)
	h.ringBytes += len(owned)
	for h.ringBytes > h.maxRingBytes && len(h.ring) > 1 {
		h.ringBytes -= len(h.ring[0].data)
		h.ring[0] = mediaChunk{}
		h.ring = h.ring[1:]
	}

	for id, sub := range h.subscribers {
		if sub.closed {
			continue
		}
		if sub.waiting {
			h.releaseWaitingLocked(id, sub, chunk)
			continue
		}
		h.enqueueLocked(id, sub, chunk)
	}
}

func (h *Hub) releaseWaitingLocked(id uint64, sub *subscriber, current mediaChunk) {
	if !h.transportSeen {
		// Preserve Hub's generic fan-out behavior for non-transport test and
		// diagnostic payloads. Real session output is always MPEG-TS.
		sub.waiting = false
		h.enqueueLocked(id, sub, current)
		return
	}
	if !h.programmeKnown {
		return
	}
	start, ok := decoderBootstrapStart(h.ring, h.hasVideo)
	if !ok {
		return
	}
	sub.waiting = false
	for _, replay := range h.ring[start:] {
		if !h.enqueueLocked(id, sub, replay) {
			return
		}
	}
}

func (h *Hub) enqueueLocked(id uint64, sub *subscriber, chunk mediaChunk) bool {
	if sub.pending+len(chunk.data) > h.maxClientBytes {
		h.dropLocked(id, sub, "slow_client_dropped")
		h.slowClientDrops.Add(1)
		return false
	}
	select {
	case sub.chunks <- chunk:
		sub.pending += len(chunk.data)
		return true
	default:
		h.dropLocked(id, sub, "slow_client_dropped")
		h.slowClientDrops.Add(1)
		return false
	}
}

// ResetForDiscontinuity removes queued bytes from the previous source before
// publishing a validated replacement. Subscribers remain registered and keep
// the same HTTP connection.
func (h *Hub) ResetForDiscontinuity() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for index := range h.ring {
		h.ring[index] = mediaChunk{}
	}
	h.ring = nil
	h.ringBytes = 0
	h.transportSeen = false
	h.programmeKnown = false
	h.hasVideo = false
	h.videoPIDs = nil
	for _, sub := range h.subscribers {
		if sub.closed {
			continue
		}
		for {
			select {
			case <-sub.chunks:
			default:
				sub.pending = 0
				goto drained
			}
		}
	drained:
		sub.waiting = true
	}
}

func (h *Hub) Subscribe() *Subscription {
	h.mu.Lock()
	defer h.mu.Unlock()

	// mpegtsmux emits seven 188-byte packets per sample. Size the channel from
	// the configured byte allowance so a cold producer's initial scheduling
	// burst cannot hit an unrelated 64/512-chunk ceiling before the HTTP writer
	// begins draining. The byte accounting remains the authoritative bound.
	queueSize := (h.maxClientBytes + canonicalTransportChunkBytes - 1) / canonicalTransportChunkBytes
	if warmCapacity := len(h.ring) + 64; queueSize < warmCapacity {
		queueSize = warmCapacity
	}
	if queueSize < 64 {
		queueSize = 64
	}
	h.nextID++
	sub := &subscriber{id: h.nextID, chunks: make(chan mediaChunk, queueSize)}
	if h.closed {
		sub.closed = true
		close(sub.chunks)
		return &Subscription{hub: h, sub: sub}
	}

	start := 0
	bootstrapReady := false
	if h.programmeKnown {
		start, bootstrapReady = decoderBootstrapStart(h.ring, h.hasVideo)
	}
	if (h.transportSeen && !bootstrapReady) || len(h.ring) == 0 {
		// Never fall back to an arbitrary mid-GOP replay. A late video viewer
		// waits for the next PAT/PMT/keyframe boundary; an audio-only viewer waits
		// only for its programme tables. This keeps the queue bounded without
		// retaining an unbounded GOP in the warm ring.
		sub.waiting = true
	} else {
		for _, chunk := range h.ring[start:] {
			if sub.pending+len(chunk.data) > h.maxClientBytes {
				break
			}
			select {
			case sub.chunks <- chunk:
				sub.pending += len(chunk.data)
			default:
				break
			}
		}
	}
	h.subscribers[sub.id] = sub
	return &Subscription{hub: h, sub: sub}
}

// HasDecoderBootstrap reports whether the warm ring can start a decoder. A
// video transport requires PAT, PMT, and a later random-access packet; an
// audio-only transport needs only its programme tables.
func (h *Hub) HasDecoderBootstrap(requireRandomAccess bool) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := decoderBootstrapStart(h.ring, requireRandomAccess)
	return ok
}

type transportChunkMetadata struct {
	isTransport      bool
	programmeKnown   bool
	hasVideo         bool
	hasPAT           bool
	hasPMT           bool
	randomAccess     bool
	randomAccessPIDs []uint16
	videoPIDs        []uint16
}

func inspectTransportChunk(data []byte) transportChunkMetadata {
	metadata := transportChunkMetadata{}
	if len(data) < 188 {
		return metadata
	}
	offset := findTSSync(data)
	if offset < 0 {
		// Small test and source buffers may contain fewer than the three packets
		// required by findTSSync. Accept only a packet-aligned sync byte here.
		for candidate := 0; candidate < 188 && candidate+188 <= len(data); candidate++ {
			if data[candidate] == 0x47 {
				offset = candidate
				break
			}
		}
	}
	if offset < 0 {
		return metadata
	}
	for index := offset; index+188 <= len(data); index += 188 {
		packet := data[index : index+188]
		if packet[0] != 0x47 {
			break
		}
		metadata.isTransport = true
		pid := uint16(packet[1]&0x1f)<<8 | uint16(packet[2])
		adaptation := (packet[3] >> 4) & 0x03
		if adaptation == 2 || adaptation == 3 {
			length := int(packet[4])
			if length > 0 && 5+length <= len(packet) && packet[5]&0x40 != 0 {
				metadata.randomAccess = true
				metadata.randomAccessPIDs = appendUniquePID(metadata.randomAccessPIDs, pid)
			}
		}
		if packet[1]&0x40 == 0 || adaptation == 0 || adaptation == 2 {
			continue
		}
		payload := 4
		if adaptation == 3 {
			payload += 1 + int(packet[4])
		}
		if payload >= len(packet) {
			continue
		}
		section := packet[payload:]
		pointer := int(section[0])
		if 1+pointer >= len(section) {
			continue
		}
		tableID := section[1+pointer]
		if pid == 0 && tableID == 0x00 {
			metadata.hasPAT = true
		}
		if tableID == 0x02 {
			metadata.hasPMT = true
			metadata.programmeKnown = true
			videoPIDs := pmtVideoPIDs(section[1+pointer:])
			metadata.hasVideo = metadata.hasVideo || len(videoPIDs) > 0
			for _, videoPID := range videoPIDs {
				metadata.videoPIDs = appendUniquePID(metadata.videoPIDs, videoPID)
			}
		}
	}
	return metadata
}

func appendUniquePID(pids []uint16, candidate uint16) []uint16 {
	for _, pid := range pids {
		if pid == candidate {
			return pids
		}
	}
	return append(pids, candidate)
}

func decoderBootstrapStart(ring []mediaChunk, requireRandomAccess bool) (int, bool) {
	if len(ring) == 0 {
		return 0, false
	}
	end := len(ring) - 1
	if requireRandomAccess {
		for end >= 0 && !ring[end].randomAccess {
			end--
		}
		if end < 0 {
			return 0, false
		}
	}
	for candidate := end; candidate >= 0; candidate-- {
		if requireRandomAccess && !ring[candidate].randomAccess {
			continue
		}
		hasPAT := false
		hasPMT := false
		for index := candidate; index >= 0; index-- {
			hasPAT = hasPAT || ring[index].hasPAT
			hasPMT = hasPMT || ring[index].hasPMT
			if hasPAT && hasPMT {
				return index, true
			}
		}
		if !requireRandomAccess {
			break
		}
	}
	return 0, false
}

func (s *Subscription) Next() ([]byte, bool) {
	s.hub.mu.Lock()
	closed := s.sub.closed
	s.hub.mu.Unlock()
	if closed {
		return nil, false
	}
	chunk, ok := <-s.sub.chunks
	if !ok {
		return nil, false
	}
	s.hub.mu.Lock()
	if s.sub.pending >= len(chunk.data) {
		s.sub.pending -= len(chunk.data)
	} else {
		s.sub.pending = 0
	}
	s.hub.mu.Unlock()
	return chunk.data, true
}

func (s *Subscription) Close() {
	s.once.Do(func() {
		s.hub.mu.Lock()
		defer s.hub.mu.Unlock()
		if current, ok := s.hub.subscribers[s.sub.id]; ok {
			s.hub.dropLocked(s.sub.id, current, "subscription_closed")
		}
	})
}

// CloseReason distinguishes a bounded slow-client eviction from ordinary
// connection teardown. It remains empty while the subscription is active.
func (s *Subscription) CloseReason() string {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	return s.sub.closeReason
}

func (h *Hub) dropLocked(id uint64, sub *subscriber, reason string) {
	if sub.closed {
		return
	}
	sub.closed = true
	sub.closeReason = reason
	delete(h.subscribers, id)
	close(sub.chunks)
}

func (h *Hub) ResetWarmBuffer() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for index := range h.ring {
		h.ring[index] = mediaChunk{}
	}
	h.ring = nil
	h.ringBytes = 0
}

func (h *Hub) Close(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	h.closeErr = err
	for id, sub := range h.subscribers {
		h.dropLocked(id, sub, "hub_closed")
	}
	h.ring = nil
	h.ringBytes = 0
}

func (h *Hub) SubscriberCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subscribers)
}

func (h *Hub) Metrics() (bytesPublished, slowClientDrops uint64) {
	return h.bytesPublished.Load(), h.slowClientDrops.Load()
}
