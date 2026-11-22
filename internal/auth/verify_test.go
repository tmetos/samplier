package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// signTestToken builds a signed RS256 JWT for the given claims.
func signTestToken(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()

	header := map[string]any{"alg": "RS256", "typ": "JWT", "kid": kid}
	hb, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}

	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(hb) + "." + enc.EncodeToString(cb)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signingInput + "." + enc.EncodeToString(sig)
}

// jwksServer serves a JWKS document containing the given public key.
func jwksServer(t *testing.T, kid string, pub *rsa.PublicKey) *httptest.Server {
	t.Helper()
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	body := map[string]any{"keys": []map[string]any{
		{"kid": kid, "kty": "RSA", "alg": "RS256", "use": "sig", "n": n, "e": e},
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestVerifyHappyPath(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	srv := jwksServer(t, "kid-1", &key.PublicKey)
	v := NewVerifier(srv.URL, "samplier")

	raw := signTestToken(t, key, "kid-1", map[string]any{
		"sub":   "user-123",
		"iss":   srv.URL,
		"aud":   []string{"account", "samplier"},
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"email": "a@b.c",
		"name":  "Ada Lovelace",
		"realm_access": map[string]any{
			"roles": []string{"viewer", "editor"},
		},
	})

	claims, err := v.Verify(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Fatalf("got subject %q", claims.Subject)
	}
	if !claims.Audience.Contains("samplier") {
		t.Fatal("expected samplier audience")
	}
	if !HasRole(claims, "editor") {
		t.Fatal("expected editor role")
	}
}

func TestVerifyExpired(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := jwksServer(t, "k", &key.PublicKey)
	v := NewVerifier(srv.URL, "")

	raw := signTestToken(t, key, "k", map[string]any{
		"sub": "u", "iss": srv.URL, "exp": time.Now().Add(-time.Hour).Unix(),
	})
	if _, err := v.Verify(context.Background(), raw); err != ErrExpired {
		t.Fatalf("got %v, want ErrExpired", err)
	}
}

func TestVerifyMissingExp(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := jwksServer(t, "k", &key.PublicKey)
	v := NewVerifier(srv.URL, "")

	raw := signTestToken(t, key, "k", map[string]any{"sub": "u", "iss": srv.URL})
	if _, err := v.Verify(context.Background(), raw); err == nil {
		t.Fatal("expected error for missing exp")
	}
}

func TestVerifyIssuerMismatch(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := jwksServer(t, "k", &key.PublicKey)
	v := NewVerifier(srv.URL, "")

	raw := signTestToken(t, key, "k", map[string]any{
		"sub": "u", "iss": "https://evil.example", "exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := v.Verify(context.Background(), raw); err == nil {
		t.Fatal("expected issuer mismatch error")
	}
}

func TestVerifyAudienceMismatch(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := jwksServer(t, "k", &key.PublicKey)
	v := NewVerifier(srv.URL, "samplier")

	raw := signTestToken(t, key, "k", map[string]any{
		"sub": "u", "iss": srv.URL, "aud": "other-client",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := v.Verify(context.Background(), raw); err == nil {
		t.Fatal("expected audience mismatch error")
	}
}

func TestVerifyBadSignature(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := jwksServer(t, "k", &key.PublicKey)
	v := NewVerifier(srv.URL, "")

	// Signed by a different key than the one published.
	raw := signTestToken(t, other, "k", map[string]any{
		"sub": "u", "iss": srv.URL, "exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := v.Verify(context.Background(), raw); err == nil {
		t.Fatal("expected signature verification failure")
	}
}

func TestVerifyUnknownKid(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := jwksServer(t, "known", &key.PublicKey)
	v := NewVerifier(srv.URL, "")

	raw := signTestToken(t, key, "unknown", map[string]any{
		"sub": "u", "iss": srv.URL, "exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := v.Verify(context.Background(), raw); err == nil {
		t.Fatal("expected unknown kid error")
	}
}

func TestVerifyBadSignatureEncoding(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := jwksServer(t, "k", &key.PublicKey)
	v := NewVerifier(srv.URL, "")

	raw := signTestToken(t, key, "k", map[string]any{
		"sub": "u", "iss": srv.URL, "exp": time.Now().Add(time.Hour).Unix(),
	})
	// Corrupt the signature segment.
	if _, err := v.Verify(context.Background(), raw+"!!!."); err == nil {
		t.Fatal("expected error for corrupted token")
	}
}

func TestMiddlewareDisabled(t *testing.T) {
	v := NewVerifier("", "")
	h := v.Middleware(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := ClaimsFrom(r.Context())
		if !ok || c.Subject != "anonymous" {
			t.Fatal("expected anonymous claims")
		}
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestMiddlewareMissingToken(t *testing.T) {
	v := NewVerifier("https://issuer", "")
	h := v.Middleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("expected WWW-Authenticate header")
	}
}

func TestMiddlewareValidToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := jwksServer(t, "k", &key.PublicKey)
	v := NewVerifier(srv.URL, "")

	raw := signTestToken(t, key, "k", map[string]any{
		"sub": "user-9", "iss": srv.URL, "exp": time.Now().Add(time.Hour).Unix(),
	})

	h := v.Middleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := ClaimsFrom(r.Context())
		if !ok || c.Subject != "user-9" {
			t.Fatal("expected verified claims")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestMiddlewareInvalidToken(t *testing.T) {
	v := NewVerifier("https://issuer.invalid", "")
	h := v.Middleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not.a.token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", rec.Code)
	}
}

func TestMiddlewareExpiredToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := jwksServer(t, "k", &key.PublicKey)
	v := NewVerifier(srv.URL, "")

	raw := signTestToken(t, key, "k", map[string]any{
		"sub": "u", "iss": srv.URL, "exp": time.Now().Add(-time.Minute).Unix(),
	})

	h := v.Middleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
	if rec.Body.String() == "" {
		t.Fatal("expected error body")
	}
}

func TestRequireRole(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Allowed.
	claims := &Claims{}
	claims.RealmAccess.Roles = []string{"admin"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(WithClaims(req.Context(), claims))
	rec := httptest.NewRecorder()
	RequireRole("admin")(handler).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}

	// Denied.
	rec = httptest.NewRecorder()
	RequireRole("admin")(handler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
}

func TestHasBearerPrefix(t *testing.T) {
	if !hasBearerPrefix("Bearer x") {
		t.Fatal("expected true")
	}
	if hasBearerPrefix("Basic x") {
		t.Fatal("expected false")
	}
}

func TestKeyCacheFallbackOnRefreshError(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	n := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())

	// Server serves keys once, then fails.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{
			{"kid": "k", "kty": "RSA", "n": n, "e": e},
		}})
	}))
	defer srv.Close()

	v := NewVerifier(srv.URL, "")
	v.ttl = 0 // force staleness immediately
	if err := v.refreshKeys(context.Background()); err != nil {
		t.Fatalf("prime: %v", err)
	}

	// After TTL the verifier refreshes, gets a 500, and falls back to cache.
	pub, err := v.key(context.Background(), "k")
	if err != nil {
		t.Fatalf("expected cached key fallback, got %v", err)
	}
	if pub == nil {
		t.Fatal("expected key")
	}
}

func TestVerifyAcceptsAuthorizedParty(t *testing.T) {
	// Keycloak sets azp to the client id and aud to "account"; the verifier
	// must accept the client via azp.
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := jwksServer(t, "k", &key.PublicKey)
	v := NewVerifier(srv.URL, "samplier-ui")

	raw := signTestToken(t, key, "k", map[string]any{
		"sub": "u", "iss": srv.URL, "aud": "account", "azp": "samplier-ui",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := v.Verify(context.Background(), raw); err != nil {
		t.Fatalf("azp should satisfy the audience check: %v", err)
	}
}

func TestVerifyRejectsWrongAuthorizedParty(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := jwksServer(t, "k", &key.PublicKey)
	v := NewVerifier(srv.URL, "samplier-ui")

	raw := signTestToken(t, key, "k", map[string]any{
		"sub": "u", "iss": srv.URL, "aud": "account", "azp": "other-client",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := v.Verify(context.Background(), raw); err == nil {
		t.Fatal("expected audience mismatch")
	}
}
