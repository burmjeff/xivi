package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func GenerateTOTPSecret() (string, error) {
	value := make([]byte, 20)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value), nil
}

func TOTPURI(secret, username string) string {
	issuer := "Xivi"
	query := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + url.PathEscape(issuer+":"+username) + "?" + query.Encode()
}

func ValidateTOTP(secret, code string, now time.Time, lastCounter int64) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return 0, false
	}
	counter := now.UTC().Unix() / 30
	for offset := int64(-1); offset <= 1; offset++ {
		candidate := counter + offset
		if candidate <= lastCounter {
			continue
		}
		if hmac.Equal([]byte(totpCode(secret, candidate)), []byte(code)) {
			return candidate, true
		}
	}
	return 0, false
}

func totpCode(secret string, counter int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return ""
	}
	message := make([]byte, 8)
	binary.BigEndian.PutUint64(message, uint64(counter))
	hash := hmac.New(sha1.New, key)
	_, _ = hash.Write(message)
	sum := hash.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1000000)
}

func GenerateRecoveryCodes(count int) ([]string, [][]byte, error) {
	if count < 1 {
		count = 10
	}
	codes := make([]string, 0, count)
	hashes := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		raw := make([]byte, 10)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, err
		}
		value := strings.ToUpper(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw))
		code := value[:4] + "-" + value[4:8] + "-" + value[8:12] + "-" + value[12:16]
		hash, err := HashToken("recovery", strings.ReplaceAll(code, "-", ""))
		if err != nil {
			return nil, nil, err
		}
		codes = append(codes, code)
		hashes = append(hashes, hash)
	}
	return codes, hashes, nil
}

func NormalizeRecoveryCode(value string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), "-", ""))
}

func ParsePositiveID(value string) (int64, bool) {
	id, err := strconv.ParseInt(value, 10, 64)
	return id, err == nil && id > 0
}
