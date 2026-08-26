package streaming

import (
	"sync"
	"sync/atomic"
)

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
	closed          bool
	closeErr        error
	bytesPublished  atomic.Uint64
	slowClientDrops atomic.Uint64
}

type subscriber struct {
	id      uint64
	chunks  chan mediaChunk
	pending int
	closed  bool
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
	chunk := mediaChunk{
		data:         owned,
		hasPAT:       metadata.hasPAT,
		hasPMT:       metadata.hasPMT,
		randomAccess: metadata.randomAccess,
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
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
		if sub.pending+len(owned) > h.maxClientBytes {
			h.dropLocked(id, sub)
			h.slowClientDrops.Add(1)
			continue
		}
		select {
		case sub.chunks <- chunk:
			sub.pending += len(owned)
		default:
			h.dropLocked(id, sub)
			h.slowClientDrops.Add(1)
		}
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
	}
}

func (h *Hub) Subscribe() *Subscription {
	h.mu.Lock()
	defer h.mu.Unlock()

	queueSize := len(h.ring) + 64
	if queueSize < 64 {
		queueSize = 64
	}
	if queueSize > 512 {
		queueSize = 512
	}
	h.nextID++
	sub := &subscriber{id: h.nextID, chunks: make(chan mediaChunk, queueSize)}
	if h.closed {
		sub.closed = true
		close(sub.chunks)
		return &Subscription{hub: h, sub: sub}
	}

	// Start a video viewer at the latest transport-table/keyframe boundary when
	// one is available. Replaying an arbitrary ring boundary can make strict
	// MPEG-TS clients consume their probe budget before seeing a decodable frame.
	// Non-TS data and audio-only streams retain the complete warm-ring fallback.
	start := 0
	if decoderStart, ok := decoderBootstrapStart(h.ring, true); ok {
		start = decoderStart
	}
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
	hasPAT       bool
	hasPMT       bool
	randomAccess bool
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
		adaptation := (packet[3] >> 4) & 0x03
		if adaptation == 2 || adaptation == 3 {
			length := int(packet[4])
			if length > 0 && 5+length <= len(packet) && packet[5]&0x40 != 0 {
				metadata.randomAccess = true
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
		pid := uint16(packet[1]&0x1f)<<8 | uint16(packet[2])
		if pid == 0 && tableID == 0x00 {
			metadata.hasPAT = true
		}
		if tableID == 0x02 {
			metadata.hasPMT = true
		}
	}
	return metadata
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
			s.hub.dropLocked(s.sub.id, current)
		}
	})
}

func (h *Hub) dropLocked(id uint64, sub *subscriber) {
	if sub.closed {
		return
	}
	sub.closed = true
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
		h.dropLocked(id, sub)
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
