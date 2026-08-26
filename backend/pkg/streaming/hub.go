package streaming

import (
	"sync"
	"sync/atomic"
)

const canonicalTransportChunkBytes = 7 * 188

type mediaChunk struct {
	data              []byte
	sequence          uint64
	hasPAT            bool
	hasPMT            bool
	decoderAccess     bool
	bootstrapSequence uint64
	bootstrapOffset   int
	accessSequence    uint64
	accessOffset      int
	patPackets        []transportPacket
	pmtPackets        []transportPacket
	pcrPackets        []transportPacket
}

type transportPacket struct {
	pid    uint16
	offset int
	data   []byte
}

type videoCodec string

const (
	videoCodecH264  videoCodec = "h264"
	videoCodecH265  videoCodec = "h265"
	videoCodecMPEG2 videoCodec = "mpeg2video"
	videoCodecMPEG4 videoCodec = "mpeg4video"
	videoCodecAVS   videoCodec = "avs"
)

type parameterSetMask uint8

const (
	parameterSetH264SPS parameterSetMask = 1 << iota
	parameterSetH264PPS
	parameterSetH265VPS
	parameterSetH265SPS
	parameterSetH265PPS
	parameterSetMPEG2Sequence
)

type elementaryStream struct {
	pid   uint16
	codec videoCodec
	video bool
	audio bool
}

type codecAccessUnit struct {
	codec           videoCodec
	sequence        uint64
	offset          int
	parameterSets   parameterSetMask
	randomAccess    bool
	randomSequence  uint64
	randomOffset    int
	complete        bool
	detectedOverall parameterSetMask
	zeroCount       int
	waitingNALType  bool
}

// BootstrapStatus describes the decoder contract offered to a newly attached
// transport client. It is persisted with the media-ready event so a failed
// tuner probe can be diagnosed without retaining raw stream data.
type BootstrapStatus struct {
	Transport             bool     `json:"transport"`
	PAT                   bool     `json:"pat"`
	PMT                   bool     `json:"pmt"`
	PCR                   bool     `json:"pcr"`
	HasVideo              bool     `json:"has_video"`
	VideoPID              uint16   `json:"video_pid,omitempty"`
	VideoCodec            string   `json:"video_codec,omitempty"`
	RandomAccess          bool     `json:"random_access"`
	RequiredParameterSets []string `json:"required_parameter_sets,omitempty"`
	DetectedParameterSets []string `json:"detected_parameter_sets,omitempty"`
	ParameterSetsComplete bool     `json:"parameter_sets_complete"`
	Complete              bool     `json:"complete"`
	Missing               []string `json:"missing,omitempty"`
	FirstVideoPTS90K      *uint64  `json:"first_video_pts_90k,omitempty"`
	FirstAudioPTS90K      *uint64  `json:"first_audio_pts_90k,omitempty"`
	AVStartDeltaMS        *int64   `json:"av_start_delta_ms,omitempty"`
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
	patSeen         bool
	pmtSeen         bool
	pcrSeen         bool
	hasVideo        bool
	videoPIDs       map[uint16]bool
	videoCodecs     map[uint16]videoCodec
	audioPIDs       map[uint16]bool
	pcrPIDs         map[uint16]bool
	codecUnits      map[uint16]*codecAccessUnit
	nextSequence    uint64
	randomSeen      bool
	firstVideoPTS   *uint64
	firstAudioPTS   *uint64
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
	h.patSeen = h.patSeen || metadata.hasPAT
	h.pmtSeen = h.pmtSeen || metadata.hasPMT
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
		if len(metadata.streams) > 0 {
			if h.videoCodecs == nil {
				h.videoCodecs = make(map[uint16]videoCodec)
			}
			if h.audioPIDs == nil {
				h.audioPIDs = make(map[uint16]bool)
			}
			for _, stream := range metadata.streams {
				if stream.video {
					h.videoCodecs[stream.pid] = stream.codec
				}
				if stream.audio {
					h.audioPIDs[stream.pid] = true
				}
			}
		}
		if len(metadata.pcrPIDs) > 0 {
			if h.pcrPIDs == nil {
				h.pcrPIDs = make(map[uint16]bool)
			}
			for _, pid := range metadata.pcrPIDs {
				h.pcrPIDs[pid] = true
			}
		}
	}
	pcrPackets := make([]transportPacket, 0, len(metadata.pcrPackets))
	for _, packet := range metadata.pcrPackets {
		if h.pcrPIDs[packet.pid] {
			pcrPackets = append(pcrPackets, packet)
		}
	}
	if len(pcrPackets) > 0 {
		h.pcrSeen = true
	}
	h.nextSequence++
	chunk := mediaChunk{
		data:       owned,
		sequence:   h.nextSequence,
		hasPAT:     metadata.hasPAT,
		hasPMT:     metadata.hasPMT,
		patPackets: metadata.patPackets,
		pmtPackets: metadata.pmtPackets,
		pcrPackets: pcrPackets,
	}
	h.inspectCodecBootstrapLocked(&chunk)

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
	plan, ok := decoderBootstrapPlan(h.ring, h.hasVideo)
	if !ok {
		return
	}
	sub.waiting = false
	if len(plan.prefix) > 0 && !h.enqueueLocked(id, sub, mediaChunk{data: plan.prefix}) {
		return
	}
	for index := plan.start; index < len(h.ring); index++ {
		replay := h.ring[index]
		if index == plan.start && plan.offset > 0 {
			replay.data = replay.data[plan.offset:]
		}
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
	h.patSeen = false
	h.pmtSeen = false
	h.pcrSeen = false
	h.hasVideo = false
	h.videoPIDs = nil
	h.videoCodecs = nil
	h.audioPIDs = nil
	h.pcrPIDs = nil
	h.codecUnits = nil
	h.randomSeen = false
	h.firstVideoPTS = nil
	h.firstAudioPTS = nil
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

	plan := decoderBootstrap{start: 0}
	bootstrapReady := false
	if h.programmeKnown {
		plan, bootstrapReady = decoderBootstrapPlan(h.ring, h.hasVideo)
	}
	if (h.transportSeen && !bootstrapReady) || len(h.ring) == 0 {
		// Never fall back to an arbitrary mid-GOP replay. A late video viewer
		// waits for the next PAT/PMT/PCR/keyframe boundary; an audio-only viewer waits
		// only for its programme tables. This keeps the queue bounded without
		// retaining an unbounded GOP in the warm ring.
		sub.waiting = true
	} else {
		if len(plan.prefix) > 0 && sub.pending+len(plan.prefix) <= h.maxClientBytes {
			sub.chunks <- mediaChunk{data: plan.prefix}
			sub.pending += len(plan.prefix)
		}
		for index := plan.start; index < len(h.ring); index++ {
			chunk := h.ring[index]
			if index == plan.start && plan.offset > 0 {
				chunk.data = chunk.data[plan.offset:]
			}
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
// recognized video transport requires PAT, PMT, a program clock reference,
// fresh codec initialization headers, and a random-access frame; an audio-only
// transport needs only its programme tables.
func (h *Hub) HasDecoderBootstrap(requireRandomAccess bool) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := decoderBootstrapPlan(h.ring, requireRandomAccess)
	return ok
}

func (h *Hub) BootstrapStatus() BootstrapStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	status := BootstrapStatus{
		Transport:    h.transportSeen,
		PAT:          h.patSeen,
		PMT:          h.pmtSeen,
		PCR:          h.pcrSeen,
		HasVideo:     h.hasVideo,
		RandomAccess: h.randomSeen,
	}
	var selectedPID uint16
	for pid := range h.videoPIDs {
		if selectedPID == 0 || pid < selectedPID {
			selectedPID = pid
		}
	}
	if selectedPID != 0 {
		codec := h.videoCodecs[selectedPID]
		status.VideoPID = selectedPID
		status.VideoCodec = string(codec)
		required := requiredParameterSets(codec)
		status.RequiredParameterSets = parameterSetNames(required)
		if unit := h.codecUnits[selectedPID]; unit != nil {
			status.DetectedParameterSets = parameterSetNames(unit.detectedOverall)
			status.ParameterSetsComplete = parameterSetsComplete(codec, unit.detectedOverall)
		} else {
			status.ParameterSetsComplete = required == 0
		}
	} else {
		status.ParameterSetsComplete = true
	}
	_, status.Complete = decoderBootstrapPlan(h.ring, h.hasVideo)
	if !status.Transport {
		status.Missing = append(status.Missing, "transport")
	}
	if !status.PAT {
		status.Missing = append(status.Missing, "pat")
	}
	if !status.PMT {
		status.Missing = append(status.Missing, "pmt")
	}
	if status.HasVideo {
		if !status.PCR {
			status.Missing = append(status.Missing, "pcr")
		}
		if !status.RandomAccess {
			status.Missing = append(status.Missing, "random_access")
		}
		if selectedPID != 0 {
			required := requiredParameterSets(h.videoCodecs[selectedPID])
			detected := parameterSetMask(0)
			if unit := h.codecUnits[selectedPID]; unit != nil {
				detected = unit.detectedOverall
			}
			for _, name := range parameterSetNames(required &^ detected) {
				status.Missing = append(status.Missing, name)
			}
		}
	}
	if h.firstVideoPTS != nil {
		value := *h.firstVideoPTS
		status.FirstVideoPTS90K = &value
	}
	if h.firstAudioPTS != nil {
		value := *h.firstAudioPTS
		status.FirstAudioPTS90K = &value
	}
	if h.firstVideoPTS != nil && h.firstAudioPTS != nil {
		delta := ptsDeltaMilliseconds(*h.firstVideoPTS, *h.firstAudioPTS)
		status.AVStartDeltaMS = &delta
	}
	return status
}

func ptsDeltaMilliseconds(video, audio uint64) int64 {
	const wrap = uint64(1) << 33
	difference := int64(video) - int64(audio)
	if difference > int64(wrap/2) {
		difference -= int64(wrap)
	} else if difference < -int64(wrap/2) {
		difference += int64(wrap)
	}
	return difference / 90
}

type transportChunkMetadata struct {
	isTransport    bool
	programmeKnown bool
	hasVideo       bool
	hasPAT         bool
	hasPMT         bool
	randomAccess   bool
	videoPIDs      []uint16
	pcrPIDs        []uint16
	patPackets     []transportPacket
	pmtPackets     []transportPacket
	pcrPackets     []transportPacket
	streams        []elementaryStream
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
			if length > 0 && 5+length <= len(packet) {
				if packet[5]&0x40 != 0 {
					metadata.randomAccess = true
				}
				if length >= 7 && packet[5]&0x10 != 0 {
					metadata.pcrPackets = append(metadata.pcrPackets,
						transportPacket{pid: pid, offset: index, data: packet})
				}
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
			metadata.patPackets = append(metadata.patPackets,
				transportPacket{pid: pid, offset: index, data: packet})
		}
		if tableID == 0x02 {
			metadata.hasPMT = true
			metadata.pmtPackets = append(metadata.pmtPackets,
				transportPacket{pid: pid, offset: index, data: packet})
			metadata.programmeKnown = true
			if pcrPID, ok := pmtPCRPID(section[1+pointer:]); ok {
				metadata.pcrPIDs = appendUniquePID(metadata.pcrPIDs, pcrPID)
			}
			streams := pmtElementaryStreams(section[1+pointer:])
			metadata.streams = append(metadata.streams, streams...)
			videoPIDs := videoPIDsFromStreams(streams)
			metadata.hasVideo = metadata.hasVideo || len(videoPIDs) > 0
			for _, videoPID := range videoPIDs {
				metadata.videoPIDs = appendUniquePID(metadata.videoPIDs, videoPID)
			}
		}
	}
	return metadata
}

func (h *Hub) inspectCodecBootstrapLocked(chunk *mediaChunk) {
	if h.codecUnits == nil {
		h.codecUnits = make(map[uint16]*codecAccessUnit)
	}
	offset := transportSyncOffset(chunk.data)
	if offset < 0 {
		return
	}
	for index := offset; index+188 <= len(chunk.data); index += 188 {
		packet := chunk.data[index : index+188]
		if packet[0] != 0x47 {
			break
		}
		pid := uint16(packet[1]&0x1f)<<8 | uint16(packet[2])
		payload := transportPayload(packet)
		if len(payload) == 0 {
			continue
		}
		payloadStart := packet[1]&0x40 != 0
		if h.audioPIDs[pid] && payloadStart && h.firstAudioPTS == nil {
			if pts, ok := parsePESPTS(payload); ok {
				value := pts
				h.firstAudioPTS = &value
			}
		}
		codec, video := h.videoCodecs[pid]
		if !video {
			continue
		}
		if payloadStart && h.firstVideoPTS == nil {
			if pts, ok := parsePESPTS(payload); ok {
				value := pts
				h.firstVideoPTS = &value
			}
		}
		unit := h.codecUnits[pid]
		if unit == nil || unit.codec != codec {
			unit = &codecAccessUnit{codec: codec}
			h.codecUnits[pid] = unit
		}
		if payloadStart {
			beginCodecAccessUnit(unit, chunk.sequence, index)
		} else if unit.sequence == 0 {
			unit.sequence = chunk.sequence
			unit.offset = index
		}
		adaptation := (packet[3] >> 4) & 0x03
		if (adaptation == 2 || adaptation == 3) && int(packet[4]) > 0 && packet[5]&0x40 != 0 {
			unit.randomAccess = true
			unit.randomSequence = chunk.sequence
			unit.randomOffset = index
			h.randomSeen = true
		}
		scanCodecParameterSets(unit, payload)
		if unit.randomAccess && !unit.complete && parameterSetsComplete(codec, unit.parameterSets) {
			unit.complete = true
			chunk.decoderAccess = true
			chunk.bootstrapSequence = unit.sequence
			chunk.bootstrapOffset = unit.offset
			chunk.accessSequence = unit.randomSequence
			chunk.accessOffset = unit.randomOffset
		}
	}
}

func beginCodecAccessUnit(unit *codecAccessUnit, sequence uint64, offset int) {
	// Parameter-only PES packets are allowed immediately before the IDR PES.
	// Everything else begins a fresh candidate so headers from an old GOP can
	// never make a later random-access packet appear independently decodable.
	carryParameters := !unit.randomAccess && !unit.complete && unit.parameterSets != 0
	detected := unit.detectedOverall
	codec := unit.codec
	parameters := unit.parameterSets
	parameterSequence := unit.sequence
	parameterOffset := unit.offset
	*unit = codecAccessUnit{codec: codec, detectedOverall: detected}
	if carryParameters {
		unit.parameterSets = parameters
		unit.sequence = parameterSequence
		unit.offset = parameterOffset
	} else {
		unit.sequence = sequence
		unit.offset = offset
	}
}

func scanCodecParameterSets(unit *codecAccessUnit, payload []byte) {
	for _, value := range payload {
		if unit.waitingNALType {
			mask := parameterSetForNAL(unit.codec, value)
			unit.parameterSets |= mask
			unit.detectedOverall |= mask
			unit.waitingNALType = false
		}
		if value == 0 {
			if unit.zeroCount < 3 {
				unit.zeroCount++
			}
			continue
		}
		if value == 1 && unit.zeroCount >= 2 {
			unit.waitingNALType = true
			unit.zeroCount = 0
			continue
		}
		unit.zeroCount = 0
	}
}

func parameterSetForNAL(codec videoCodec, nalHeader byte) parameterSetMask {
	switch codec {
	case videoCodecH264:
		switch nalHeader & 0x1f {
		case 7:
			return parameterSetH264SPS
		case 8:
			return parameterSetH264PPS
		}
	case videoCodecH265:
		switch (nalHeader >> 1) & 0x3f {
		case 32:
			return parameterSetH265VPS
		case 33:
			return parameterSetH265SPS
		case 34:
			return parameterSetH265PPS
		}
	case videoCodecMPEG2:
		if nalHeader == 0xb3 {
			return parameterSetMPEG2Sequence
		}
	}
	return 0
}

func requiredParameterSets(codec videoCodec) parameterSetMask {
	switch codec {
	case videoCodecH264:
		return parameterSetH264SPS | parameterSetH264PPS
	case videoCodecH265:
		return parameterSetH265VPS | parameterSetH265SPS | parameterSetH265PPS
	case videoCodecMPEG2:
		return parameterSetMPEG2Sequence
	default:
		return 0
	}
}

func parameterSetsComplete(codec videoCodec, detected parameterSetMask) bool {
	required := requiredParameterSets(codec)
	return required == 0 || detected&required == required
}

func parameterSetNames(mask parameterSetMask) []string {
	names := make([]string, 0, 3)
	for _, item := range []struct {
		mask parameterSetMask
		name string
	}{
		{parameterSetH264SPS, "sps"},
		{parameterSetH264PPS, "pps"},
		{parameterSetH265VPS, "vps"},
		{parameterSetH265SPS, "sps"},
		{parameterSetH265PPS, "pps"},
		{parameterSetMPEG2Sequence, "sequence_header"},
	} {
		if mask&item.mask != 0 {
			names = append(names, item.name)
		}
	}
	return names
}

func transportSyncOffset(data []byte) int {
	offset := findTSSync(data)
	if offset >= 0 {
		return offset
	}
	for candidate := 0; candidate < 188 && candidate+188 <= len(data); candidate++ {
		if data[candidate] == 0x47 {
			return candidate
		}
	}
	return -1
}

func transportPayload(packet []byte) []byte {
	if len(packet) != 188 || packet[0] != 0x47 {
		return nil
	}
	adaptation := (packet[3] >> 4) & 0x03
	if adaptation == 0 || adaptation == 2 {
		return nil
	}
	offset := 4
	if adaptation == 3 {
		offset += 1 + int(packet[4])
	}
	if offset >= len(packet) {
		return nil
	}
	return packet[offset:]
}

func parsePESPTS(payload []byte) (uint64, bool) {
	if len(payload) < 14 || payload[0] != 0 || payload[1] != 0 || payload[2] != 1 || payload[7]&0x80 == 0 {
		return 0, false
	}
	pts := uint64(payload[9]>>1&0x07) << 30
	pts |= uint64(payload[10]) << 22
	pts |= uint64(payload[11]>>1&0x7f) << 15
	pts |= uint64(payload[12]) << 7
	pts |= uint64(payload[13]>>1) & 0x7f
	return pts, true
}

func pmtPCRPID(section []byte) (uint16, bool) {
	if len(section) < 12 || section[0] != 0x02 {
		return 0, false
	}
	pcrPID := uint16(section[8]&0x1f)<<8 | uint16(section[9])
	if pcrPID == 0x1fff {
		return 0, false
	}
	return pcrPID, true
}

func appendUniquePID(pids []uint16, candidate uint16) []uint16 {
	for _, pid := range pids {
		if pid == candidate {
			return pids
		}
	}
	return append(pids, candidate)
}

type decoderBootstrap struct {
	start  int
	offset int
	prefix []byte
}

// decoderBootstrapPlan starts video replay at the PES/access-unit boundary that
// contains fresh codec initialization headers and a random-access frame.
// Programme tables and a payload-free program clock reference are copied into
// a compact prefix. This gives strict tuner probes SPS/PPS (or their codec
// equivalent) without restoring an old GOP or putting audio seconds ahead.
func decoderBootstrapPlan(ring []mediaChunk, requireRandomAccess bool) (decoderBootstrap, bool) {
	if len(ring) == 0 {
		return decoderBootstrap{}, false
	}
	end := len(ring) - 1
	if requireRandomAccess {
		for end >= 0 && !ring[end].decoderAccess {
			end--
		}
		if end < 0 {
			return decoderBootstrap{}, false
		}
		start := -1
		for index := end; index >= 0; index-- {
			if ring[index].sequence == ring[end].bootstrapSequence {
				start = index
				break
			}
		}
		if start < 0 {
			return decoderBootstrap{}, false
		}
		bootstrapOffset := ring[end].bootstrapOffset
		var pat []byte
		var pmt []byte
		var pcr []byte
		for index := start; index >= 0 && (len(pat) == 0 || len(pmt) == 0); index-- {
			limit := len(ring[index].data)
			if index == start {
				limit = bootstrapOffset
			}
			if len(pat) == 0 {
				pat = latestPacketBefore(ring[index].patPackets, limit)
			}
			if len(pmt) == 0 {
				pmt = latestPacketBefore(ring[index].pmtPackets, limit)
			}
		}
		for index := end; index >= 0 && len(pcr) == 0; index-- {
			limit := len(ring[index].data)
			if ring[index].sequence == ring[end].accessSequence {
				// A PCR on the random-access packet is safe to copy as an
				// adaptation-only discontinuity immediately before the AU.
				limit = ring[end].accessOffset + 1
			} else if ring[index].sequence > ring[end].accessSequence {
				continue
			}
			pcr = latestPacketBefore(ring[index].pcrPackets, limit)
		}
		if len(pat) == 0 || len(pmt) == 0 || len(pcr) == 0 {
			return decoderBootstrap{}, false
		}
		clock := bootstrapPCRPacket(pcr)
		if len(clock) == 0 {
			return decoderBootstrap{}, false
		}
		prefix := make([]byte, 0, len(pat)+len(pmt)+len(clock))
		prefix = append(prefix, pat...)
		prefix = append(prefix, pmt...)
		prefix = append(prefix, clock...)
		return decoderBootstrap{start: start, offset: bootstrapOffset, prefix: prefix}, true
	}
	for candidate := end; candidate >= 0; candidate-- {
		hasPAT := false
		hasPMT := false
		for index := candidate; index >= 0; index-- {
			hasPAT = hasPAT || ring[index].hasPAT
			hasPMT = hasPMT || ring[index].hasPMT
			if hasPAT && hasPMT {
				return decoderBootstrap{start: index}, true
			}
		}
		break
	}
	return decoderBootstrap{}, false
}

// bootstrapPCRPacket copies only the clock from a source packet. Replaying the
// original packet could include delta-video payload from before the selected
// IDR. Adaptation-only packets do not advance payload continuity, and the
// discontinuity flag tells a new demuxer to establish a fresh clock baseline.
func bootstrapPCRPacket(source []byte) []byte {
	if len(source) != 188 || source[0] != 0x47 {
		return nil
	}
	adaptation := (source[3] >> 4) & 0x03
	if adaptation != 2 && adaptation != 3 {
		return nil
	}
	length := int(source[4])
	if length < 7 || 5+length > len(source) || source[5]&0x10 == 0 {
		return nil
	}
	packet := make([]byte, 188)
	for index := range packet {
		packet[index] = 0xff
	}
	copy(packet[:3], source[:3])
	packet[1] &^= 0x40
	packet[3] = source[3]&0x0f | 0x20
	packet[4] = 183
	packet[5] = 0x90
	copy(packet[6:12], source[6:12])
	return packet
}

func latestPacketBefore(packets []transportPacket, limit int) []byte {
	for index := len(packets) - 1; index >= 0; index-- {
		if packets[index].offset < limit {
			return packets[index].data
		}
	}
	return nil
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
