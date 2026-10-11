package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"krampus/server/config"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testIssuer   = "https://idp.example.com"
	testClientID = "krampus-client"
	testSecret   = "hs256-shared-secret"
)

// setupOIDCTest points the globals at a test config with no live provider
// (discovery off) and restores them afterwards.
func setupOIDCTest(t *testing.T, allowHS256 bool) {
	t.Helper()
	prevConfig := config.AppConfig
	prevProvider := OIDCProvider
	t.Cleanup(func() {
		config.AppConfig = prevConfig
		OIDCProvider = prevProvider
	})
	config.AppConfig = &config.Config{
		OIDCProviderURL:  testIssuer,
		OIDCClientID:     testClientID,
		OIDCClientSecret: testSecret,
		OIDCAllowHS256:   allowHS256,
	}
	OIDCProvider = nil
}

func mintHMACToken(t *testing.T, method jwt.SigningMethod, claims jwt.MapClaims) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(method, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss": testIssuer,
		"aud": testClientID,
		"sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
}

func TestHS256RejectedWhenFlagOff(t *testing.T) {
	setupOIDCTest(t, false)
	raw := mintHMACToken(t, jwt.SigningMethodHS256, validClaims())
	_, err := VerifyIDToken(context.Background(), raw)
	if err == nil {
		t.Fatal("HMAC-signed token accepted with OIDCAllowHS256=false")
	}
	if strings.Contains(err.Error(), "HS256 token verification") {
		t.Errorf("fallback was attempted despite flag off: %v", err)
	}
}

func TestHS256FallbackPinsAlgorithm(t *testing.T) {
	setupOIDCTest(t, true)
	// HS384 is still in the HMAC family; the old keyfunc accepted it. The
	// pinned fallback must reject anything whose header alg is not HS256.
	raw := mintHMACToken(t, jwt.SigningMethodHS384, validClaims())
	if _, err := verifyHS256(raw, testIssuer); err == nil {
		t.Fatal("HS384-signed token accepted by HS256 fallback")
	}
}

func TestHS256FallbackValidToken(t *testing.T) {
	setupOIDCTest(t, true)
	raw := mintHMACToken(t, jwt.SigningMethodHS256, validClaims())
	idToken, err := verifyHS256(raw, testIssuer)
	if err != nil {
		t.Fatalf("valid HS256 token rejected: %v", err)
	}
	var claims IDTokenClaims
	if err := idToken.Claims(&claims); err != nil {
		t.Fatalf("claims unmarshal: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Errorf("sub=%q, want user-1", claims.Subject)
	}
}

func TestHS256FallbackRejectsWrongAudience(t *testing.T) {
	setupOIDCTest(t, true)
	claims := validClaims()
	claims["aud"] = "some-other-client"
	raw := mintHMACToken(t, jwt.SigningMethodHS256, claims)
	if _, err := verifyHS256(raw, testIssuer); err == nil {
		t.Fatal("token with wrong audience accepted")
	}
}

func TestHS256FallbackRejectsWrongIssuer(t *testing.T) {
	setupOIDCTest(t, true)
	claims := validClaims()
	claims["iss"] = "https://evil.example.com"
	raw := mintHMACToken(t, jwt.SigningMethodHS256, claims)
	if _, err := verifyHS256(raw, testIssuer); err == nil {
		t.Fatal("token with wrong issuer accepted")
	}
}

func TestHS256FallbackRejectsExpired(t *testing.T) {
	setupOIDCTest(t, true)
	claims := validClaims()
	claims["exp"] = time.Now().Add(-time.Hour).Unix()
	raw := mintHMACToken(t, jwt.SigningMethodHS256, claims)
	if _, err := verifyHS256(raw, testIssuer); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestDiscoveredIssuerFallsBackToConfig(t *testing.T) {
	setupOIDCTest(t, true)
	if got := discoveredIssuer(); got != testIssuer {
		t.Errorf("discoveredIssuer()=%q, want configured %q", got, testIssuer)
	}
}
