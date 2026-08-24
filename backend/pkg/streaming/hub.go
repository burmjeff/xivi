package streaming

import (
	"sync"
	"sync/atomic"
)

type mediaChunk struct {
	data []byte
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
	if len(data) == 0 {
		return
	}
	owned := append([]byte(nil), data...)
	chunk := mediaChunk{data: owned}

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

	// A short warm ring gives a new viewer recent transport tables and media
	// without increasing latency for existing viewers.
	for _, chunk := range h.ring {
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
