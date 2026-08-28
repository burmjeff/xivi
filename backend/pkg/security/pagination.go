package security

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

const pageCursorLifetime = 30 * time.Minute

type pageCursor struct {
	Version int    `json:"v"`
	Offset  int    `json:"o"`
	Expires int64  `json:"e"`
	Binding string `json:"b"`
}

// EncodePageCursor makes pagination state confidential and tamper evident. Binding is
// supplied by the controller and includes the authenticated user, route, and
// non-cursor query parameters, so a cursor cannot be moved to another account,
// lineup, filter, or endpoint.
func EncodePageCursor(offset int, binding string) (string, error) {
	if offset < 0 || binding == "" {
		return "", errors.New("invalid page cursor")
	}
	payload, err := json.Marshal(pageCursor{Version: 1, Offset: offset, Expires: time.Now().Add(pageCursorLifetime).Unix(), Binding: binding})
	if err != nil {
		return "", err
	}
	sealed, err := EncryptSecret(payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func ParsePageCursor(value, binding string) (int, error) {
	if binding == "" || len(value) > 2048 {
		return 0, errors.New("invalid page cursor")
	}
	sealed, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(sealed) > 1024 {
		return 0, errors.New("invalid page cursor")
	}
	payload, err := DecryptSecret(sealed)
	if err != nil {
		return 0, errors.New("invalid page cursor")
	}
	decoded := pageCursor{}
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded.Version != 1 || decoded.Offset < 0 ||
		decoded.Binding != binding || time.Now().Unix() >= decoded.Expires {
		return 0, errors.New("invalid page cursor")
	}
	return decoded.Offset, nil
}
