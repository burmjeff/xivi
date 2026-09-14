package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/url"
	"strings"
	"time"
)

var ErrDPoP = errors.New("invalid DPoP proof")

type DPoPProof struct {
	Thumbprint string
	JTI        string
	Nonce      string
}

type dpopJWK struct {
	Crv string `json:"crv"`
	Kty string `json:"kty"`
	X   string `json:"x"`
	Y   string `json:"y"`
	D   string `json:"d,omitempty"`
}

// DPoPTarget performs RFC 9449 URI comparison without query or fragment. The
// caller must derive the external origin only from its trusted proxy policy.
func DPoPTarget(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Opaque != "" {
		return "", ErrDPoP
	}
	u.Scheme, u.Host = "https", strings.ToLower(u.Host)
	if u.Port() == "443" {
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}
	u.RawQuery, u.Fragment, u.RawFragment, u.ForceQuery = "", "", "", false
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

// VerifyDPoP validates an ES256 public-client proof. The caller MUST atomically
// consume (Thumbprint,JTI) and validate a server nonce for token requests.
func VerifyDPoP(raw, method, target, expectedThumbprint, accessToken string, now time.Time) (DPoPProof, error) {
	bad := func() (DPoPProof, error) { return DPoPProof{}, ErrDPoP }
	if len(raw) > 8192 {
		return bad()
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return bad()
	}
	decode := base64.RawURLEncoding.DecodeString
	headerBytes, err := decode(parts[0])
	if err != nil {
		return bad()
	}
	payloadBytes, err := decode(parts[1])
	if err != nil {
		return bad()
	}
	signature, err := decode(parts[2])
	if err != nil || len(signature) != 64 {
		return bad()
	}
	var header struct {
		Typ  string   `json:"typ"`
		Alg  string   `json:"alg"`
		JWK  dpopJWK  `json:"jwk"`
		Crit []string `json:"crit"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || header.Typ != "dpop+jwt" || header.Alg != "ES256" ||
		len(header.Crit) != 0 || header.JWK.Crv != "P-256" || header.JWK.Kty != "EC" || header.JWK.D != "" {
		return bad()
	}
	x, err := decode(header.JWK.X)
	if err != nil || len(x) != 32 {
		return bad()
	}
	y, err := decode(header.JWK.Y)
	if err != nil || len(y) != 32 {
		return bad()
	}
	key := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
	if !key.Curve.IsOnCurve(key.X, key.Y) {
		return bad()
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
		return bad()
	}
	canonical, _ := json.Marshal(header.JWK)
	keyHash := sha256.Sum256(canonical)
	thumb := base64.RawURLEncoding.EncodeToString(keyHash[:])
	if expectedThumbprint != "" && subtle.ConstantTimeCompare([]byte(thumb), []byte(expectedThumbprint)) != 1 {
		return bad()
	}
	var claims struct {
		JTI   string `json:"jti"`
		HTM   string `json:"htm"`
		HTU   string `json:"htu"`
		IAT   int64  `json:"iat"`
		ATH   string `json:"ath"`
		Nonce string `json:"nonce"`
	}
	if json.Unmarshal(payloadBytes, &claims) != nil || len(claims.JTI) < 16 || len(claims.JTI) > 128 ||
		claims.HTM != method || claims.IAT < now.Add(-5*time.Minute).Unix() || claims.IAT > now.Add(time.Minute).Unix() {
		return bad()
	}
	want, err := DPoPTarget(target)
	if err != nil {
		return bad()
	}
	got, err := DPoPTarget(claims.HTU)
	if err != nil || want != got {
		return bad()
	}
	// htu itself must omit query/fragment, even though comparison normalizes the target.
	u, _ := url.Parse(claims.HTU)
	if u.RawQuery != "" || u.Fragment != "" {
		return bad()
	}
	if accessToken != "" {
		hash := sha256.Sum256([]byte(accessToken))
		if subtle.ConstantTimeCompare([]byte(claims.ATH), []byte(base64.RawURLEncoding.EncodeToString(hash[:]))) != 1 {
			return bad()
		}
	}
	return DPoPProof{Thumbprint: thumb, JTI: claims.JTI, Nonce: claims.Nonce}, nil
}

func NewDPoPNonce(thumb string, now time.Time) (string, error) {
	random, err := RandomToken(16)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(struct {
		Thumb   string
		Expires int64
		Random  string
	}{thumb, now.Add(5 * time.Minute).Unix(), random})
	sealed, err := EncryptSecret(data)
	return base64.RawURLEncoding.EncodeToString(sealed), err
}

func ValidDPoPNonce(nonce, thumb string, now time.Time) bool {
	if len(nonce) > 1024 {
		return false
	}
	sealed, err := base64.RawURLEncoding.DecodeString(nonce)
	if err != nil {
		return false
	}
	data, err := DecryptSecret(sealed)
	if err != nil {
		return false
	}
	var value struct {
		Thumb   string
		Expires int64
		Random  string
	}
	return json.Unmarshal(data, &value) == nil && value.Thumb == thumb && value.Expires > now.Unix() && value.Expires <= now.Add(5*time.Minute).Unix() && value.Random != ""
}
