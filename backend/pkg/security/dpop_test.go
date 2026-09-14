package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func testDPoP(t *testing.T, key *ecdsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	b64 := base64.RawURLEncoding.EncodeToString
	header, _ := json.Marshal(map[string]any{"typ": "dpop+jwt", "alg": "ES256", "jwk": dpopJWK{
		Crv: "P-256", Kty: "EC", X: b64(key.X.FillBytes(make([]byte, 32))), Y: b64(key.Y.FillBytes(make([]byte, 32)))}})
	payload, _ := json.Marshal(claims)
	signed := b64(header) + "." + b64(payload)
	hash := sha256.Sum256([]byte(signed))
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return signed + "." + b64(append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...))
}

func TestDPoPRejectsWrongBindingMethodURLTokenAndTime(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Now().UTC()
	hash := sha256.Sum256([]byte("access"))
	claims := map[string]any{"jti": "unique-request-123456", "htm": "GET", "htu": "https://tv.example/images/logo.png", "iat": now.Unix(), "ath": base64.RawURLEncoding.EncodeToString(hash[:])}
	proof := testDPoP(t, key, claims)
	verified, err := VerifyDPoP(proof, "GET", "https://tv.example/images/logo.png?lineup_id=1", "", "access", now)
	if err != nil || verified.Thumbprint == "" {
		t.Fatalf("valid proof: %v", err)
	}
	for _, tc := range []struct {
		method, target, thumb, token string
		at                           time.Time
	}{
		{"POST", "https://tv.example/images/logo.png", "", "access", now},
		{"GET", "https://other.example/images/logo.png", "", "access", now},
		{"GET", "https://tv.example/images/logo.png", "wrong", "access", now},
		{"GET", "https://tv.example/images/logo.png", "", "different", now},
		{"GET", "https://tv.example/images/logo.png", "", "access", now.Add(6 * time.Minute)},
	} {
		if _, err := VerifyDPoP(proof, tc.method, tc.target, tc.thumb, tc.token, tc.at); err == nil {
			t.Fatalf("accepted invalid proof: %+v", tc)
		}
	}
	claims["htu"] = "https://tv.example/images/logo.png?secret=value"
	if _, err := VerifyDPoP(testDPoP(t, key, claims), "GET", "https://tv.example/images/logo.png", "", "access", now); err == nil {
		t.Fatal("accepted query in htu")
	}
}

func TestDPoPNonceBindingAndExpiry(t *testing.T) {
	initializeTestKey(t)
	now := time.Now().UTC()
	nonce, err := NewDPoPNonce("device", now)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidDPoPNonce(nonce, "device", now) || ValidDPoPNonce(nonce, "other", now) || ValidDPoPNonce(nonce, "device", now.Add(6*time.Minute)) {
		t.Fatal("nonce binding or expiry failed")
	}
}
