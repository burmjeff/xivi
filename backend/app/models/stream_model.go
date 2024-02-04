package models

type LiveChannel struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Logo      string `json:"logo"`
	Stream    string `json:"stream"`
	Programme string `json:"programme"`
	Start     string `json:"start"`
	End       string `json:"end"`
	Next      string `json:"next"`
}
