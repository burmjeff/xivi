package models

type Settings struct {
	Proxy      bool `json:"proxy,omitempty"`
	Buffer     bool `json:"buffer,omitempty"`
	BufferTime int  `json:"buffertime,omitempty"`
}
