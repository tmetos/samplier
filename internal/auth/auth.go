// Package auth implements OIDC bearer-token authentication against Keycloak
// (or any OIDC-compliant provider) using JWT validation and JWKS discovery.
package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	// ErrNoToken indicates a missing bearer token.
	ErrNoToken = errors.New("no bearer token")
	// ErrInvalidToken indicates a malformed or unverifiable token.
	ErrInvalidToken = errors.New("invalid token")
	// ErrExpired indicates an expired token.
	ErrExpired = errors.New("token expired")
)

// Claims holds the JWT claims Samplier cares about.
type Claims struct {
	Subject           string   `json:"sub"`
	Issuer            string   `json:"iss"`
	Audience          Audience `json:"aud"`
	Expiry            int64    `json:"exp"`
	IssuedAt          int64    `json:"iat"`
	Email             string   `json:"email"`
	DisplayName       string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	// AuthorizedParty is the client the token was issued to. Keycloak sets it
	// to the client id while `aud` is often the generic "account".
	AuthorizedParty string `json:"azp"`
	// Roles come from Keycloak's realm_access.roles.
	RealmAccess struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
}

// Audience accepts either a string or an array of strings.
type Audience []string

// UnmarshalJSON handles both `"aud": "x"` and `"aud": ["x","y"]`.
func (a *Audience) UnmarshalJSON(b []byte) error {
	var single string
	if err := json.Unmarshal(b, &single); err == nil {
		*a = Audience{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*a = Audience(many)
	return nil
}

// Contains reports whether the audience list includes v.
func (a Audience) Contains(v string) bool {
	for _, x := range a {
		if x == v {
			return true
		}
	}
	return false
}

// Verifier validates JWTs against an issuer's JWKS endpoint.
type Verifier struct {
	issuer   string
	audience string // optional; empty skips audience validation

	mu         sync.RWMutex
	keys       map[string]*rsa.PublicKey
	fetchedAt  time.Time
	httpClient *http.Client
	ttl        time.Duration
}

// NewVerifier constructs a Verifier. The JWKS is fetched lazily and cached.
func NewVerifier(issuer, audience string) *Verifier {
	return &Verifier{
		issuer:     strings.TrimSuffix(issuer, "/"),
		audience:   audience,
		keys:       map[string]*rsa.PublicKey{},
		httpClient: &http.Client{Timeout: 10 * time.Second},
		ttl:        15 * time.Minute,
	}
}

// Issuer returns the configured issuer.
func (v *Verifier) Issuer() string { return v.issuer }

type jwksDoc struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		Alg string `json:"alg"`
		Use string `json:"use"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

// refreshKeys fetches the issuer's JWKS document.
func (v *Verifier) refreshKeys(ctx context.Context) error {
	if v.issuer == "" {
		return fmt.Errorf("%w: no issuer configured", ErrInvalidToken)
	}
	url := v.issuer + "/protocol/openid-connect/certs"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks fetch failed: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	var doc jwksDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return err
	}

	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if !strings.EqualFold(k.Kty, "RSA") {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		pub := &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(new(big.Int).SetBytes(eBytes).Int64()),
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return fmt.Errorf("%w: no usable signing keys", ErrInvalidToken)
	}

	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	pub, ok := v.keys[kid]
	stale := time.Since(v.fetchedAt) > v.ttl
	v.mu.RUnlock()

	if ok && !stale {
		return pub, nil
	}
	if err := v.refreshKeys(ctx); err != nil {
		if ok {
			return pub, nil // fall back to cached key on refresh failure
		}
		return nil, err
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	pub, ok = v.keys[kid]
	if !ok {
		return nil, fmt.Errorf("%w: unknown signing key %q", ErrInvalidToken, kid)
	}
	return pub, nil
}

// joseHeader is the decoded JWT header.
type joseHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

// Verify validates a raw bearer token and returns its claims.
func (v *Verifier) Verify(ctx context.Context, raw string) (*Claims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: expected 3 segments", ErrInvalidToken)
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: bad header encoding", ErrInvalidToken)
	}
	var header joseHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("%w: bad header JSON", ErrInvalidToken)
	}
	if !strings.EqualFold(header.Alg, "RS256") {
		return nil, fmt.Errorf("%w: unsupported alg %q", ErrInvalidToken, header.Alg)
	}

	pub, err := v.key(ctx, header.Kid)
	if err != nil {
		return nil, err
	}

	signingInput := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: bad signature encoding", ErrInvalidToken)
	}
	digest := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		return nil, fmt.Errorf("%w: signature verification failed", ErrInvalidToken)
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: bad payload encoding", ErrInvalidToken)
	}
	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("%w: bad payload JSON", ErrInvalidToken)
	}

	if claims.Issuer != v.issuer {
		return nil, fmt.Errorf("%w: issuer mismatch", ErrInvalidToken)
	}
	if v.audience != "" && !claims.Audience.Contains(v.audience) &&
		!strings.EqualFold(claims.AuthorizedParty, v.audience) {
		return nil, fmt.Errorf("%w: audience mismatch (aud=%v azp=%q)",
			ErrInvalidToken, claims.Audience, claims.AuthorizedParty)
	}
	if claims.Expiry == 0 {
		return nil, fmt.Errorf("%w: missing exp", ErrInvalidToken)
	}
	if time.Now().After(time.Unix(claims.Expiry, 0)) {
		return nil, ErrExpired
	}
	return &claims, nil
}

// BearerToken extracts a bearer token from an Authorization header value.
func BearerToken(header string) (string, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", ErrNoToken
	}
	const prefix = "bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", ErrNoToken
	}
	tok := strings.TrimSpace(header[len(prefix):])
	if tok == "" {
		return "", ErrNoToken
	}
	return tok, nil
}

// contextKey is an unexported type for context keys in this package.
type contextKey struct{ name string }

var claimsKey = contextKey{"claims"}

// WithClaims returns a context carrying the verified claims.
func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// ClaimsFrom returns verified claims from the context, if present.
func ClaimsFrom(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*Claims)
	return c, ok
}

// HasRole reports whether the authenticated user holds the given realm role.
func HasRole(c *Claims, role string) bool {
	if c == nil {
		return false
	}
	for _, r := range c.RealmAccess.Roles {
		if r == role {
			return true
		}
	}
	return false
}
