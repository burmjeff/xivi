package streaming

// tsProbe waits for stable 188-byte transport packets plus PAT and PMT tables.
// Requiring real transport metadata avoids reporting a pipeline as ready just
// because an upstream server returned an HTML error body or partial bytes.
type tsProbe struct {
	buffer []byte
	offset int
	pmtPID uint16
	pat    bool
	pmt    bool
	video  bool
}

func (p *tsProbe) Push(data []byte) bool {
	if p.pat && p.pmt {
		return true
	}
	if len(p.buffer) < 2*1024*1024 {
		remaining := 2*1024*1024 - len(p.buffer)
		if len(data) > remaining {
			data = data[:remaining]
		}
		p.buffer = append(p.buffer, data...)
	}
	if p.offset == 0 {
		p.offset = findTSSync(p.buffer)
		if p.offset < 0 {
			p.offset = 0
			return false
		}
	}
	for index := p.offset; index+188 <= len(p.buffer); index += 188 {
		packet := p.buffer[index : index+188]
		if packet[0] != 0x47 {
			continue
		}
		pid := uint16(packet[1]&0x1f)<<8 | uint16(packet[2])
		payloadStart := packet[1]&0x40 != 0
		adaptation := (packet[3] >> 4) & 0x03
		if adaptation == 0 || adaptation == 2 {
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
		if payloadStart {
			pointer := int(section[0])
			if 1+pointer >= len(section) {
				continue
			}
			section = section[1+pointer:]
		}
		if pid == 0 && payloadStart {
			if candidate, ok := parsePAT(section); ok {
				p.pat = true
				p.pmtPID = candidate
			}
		}
		if p.pat && pid == p.pmtPID && payloadStart && len(section) >= 3 && section[0] == 0x02 {
			p.pmt = true
			p.video = pmtHasVideo(section)
		}
	}
	return p.pat && p.pmt
}

func (p *tsProbe) HasVideo() bool {
	return p.video
}

func findTSSync(data []byte) int {
	for offset := 0; offset < 188 && offset+376 < len(data); offset++ {
		if data[offset] == 0x47 && data[offset+188] == 0x47 && data[offset+376] == 0x47 {
			return offset
		}
	}
	return -1
}

func parsePAT(section []byte) (uint16, bool) {
	if len(section) < 12 || section[0] != 0x00 {
		return 0, false
	}
	sectionLength := int(section[1]&0x0f)<<8 | int(section[2])
	end := 3 + sectionLength - 4
	if end > len(section) {
		end = len(section)
	}
	for index := 8; index+4 <= end; index += 4 {
		program := uint16(section[index])<<8 | uint16(section[index+1])
		if program == 0 {
			continue
		}
		pid := uint16(section[index+2]&0x1f)<<8 | uint16(section[index+3])
		return pid, true
	}
	return 0, false
}

func pmtHasVideo(section []byte) bool {
	return len(pmtVideoPIDs(section)) > 0
}

func pmtVideoPIDs(section []byte) []uint16 {
	return videoPIDsFromStreams(pmtElementaryStreams(section))
}

func videoPIDsFromStreams(streams []elementaryStream) []uint16 {
	videoPIDs := make([]uint16, 0, 1)
	for _, stream := range streams {
		if stream.video {
			videoPIDs = append(videoPIDs, stream.pid)
		}
	}
	return videoPIDs
}

func pmtElementaryStreams(section []byte) []elementaryStream {
	if len(section) < 12 || section[0] != 0x02 {
		return nil
	}
	sectionLength := int(section[1]&0x0f)<<8 | int(section[2])
	end := 3 + sectionLength - 4
	if end > len(section) {
		end = len(section)
	}
	programInfoLength := int(section[10]&0x0f)<<8 | int(section[11])
	streams := make([]elementaryStream, 0, 2)
	for index := 12 + programInfoLength; index+5 <= end; {
		streamType := section[index]
		stream := elementaryStream{
			pid: uint16(section[index+1]&0x1f)<<8 | uint16(section[index+2]),
		}
		switch streamType {
		case 0x01, 0x02:
			stream.video = true
			stream.codec = videoCodecMPEG2
		case 0x10:
			stream.video = true
			stream.codec = videoCodecMPEG4
		case 0x1b:
			stream.video = true
			stream.codec = videoCodecH264
		case 0x24:
			stream.video = true
			stream.codec = videoCodecH265
		case 0x42:
			stream.video = true
			stream.codec = videoCodecAVS
		case 0x03, 0x04, 0x0f, 0x11, 0x81, 0x87:
			stream.audio = true
		}
		streams = append(streams, stream)
		esInfoLength := int(section[index+3]&0x0f)<<8 | int(section[index+4])
		index += 5 + esInfoLength
	}
	return streams
}
