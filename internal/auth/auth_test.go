package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerToken(t *testing.T) {
	cases := []struct {
		header  string
		want    string
		wantErr bool
	}{
		{"Bearer abc123", "abc123", false},
		{"bearer abc123", "abc123", false},
		{"BEARER abc123", "abc123", false},
		{"Bearer   abc123  ", "abc123", false},
		{"", "", true},
		{"Basic abc123", "", true},
		{"Bearer ", "", true},
		{"Bearer", "", true},
	}
	for _, tc := range cases {
		got, err := BearerToken(tc.header)
		if tc.wantErr {
			if err == nil {
				t.Errorf("BearerToken(%q) expected error", tc.header)
			}
			continue
		}
		if err != nil {
			t.Errorf("BearerToken(%q) unexpected error: %v", tc.header, err)
			continue
		}
		if got != tc.want {
			t.Errorf("BearerToken(%q) = %q, want %q", tc.header, got, tc.want)
		}
	}
}

func TestAudienceUnmarshal(t *testing.T) {
	var a Audience
	if err := a.UnmarshalJSON([]byte(`"samplier"`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !a.Contains("samplier") || a.Contains("other") {
		t.Fatalf("unexpected audience: %v", a)
	}

	var b Audience
	if err := b.UnmarshalJSON([]byte(`["a","samplier"]`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !b.Contains("samplier") || !b.Contains("a") {
		t.Fatalf("unexpected audience: %v", b)
	}

	var c Audience
	if err := c.UnmarshalJSON([]byte(`123`)); err == nil {
		t.Fatal("expected error for numeric audience")
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	v := NewVerifier("https://issuer.example/realms/samplier", "samplier")
	ctx := context.Background()

	for _, tok := range []string{"", "abc", "a.b", "a.b.c.d"} {
		if _, err := v.Verify(ctx, tok); err == nil {
			t.Errorf("Verify(%q) expected error", tok)
		}
	}
}

func TestVerifyRejectsBadHeader(t *testing.T) {
	v := NewVerifier("https://issuer.example/realms/samplier", "samplier")
	ctx := context.Background()

	// "abc" base64url decodes to non-JSON.
	if _, err := v.Verify(ctx, "abc.def.ghi"); err == nil {
		t.Fatal("expected error for non-JSON header")
	}
}

func TestVerifyRejectsUnsupportedAlg(t *testing.T) {
	v := NewVerifier("https://issuer.example/realms/samplier", "samplier")
	// {"alg":"none","typ":"JWT"}
	header := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0"
	if _, err := v.Verify(context.Background(), header+".e30.sig"); err == nil {
		t.Fatal("expected error for alg=none")
	}
}

func TestRefreshKeysFromServer(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"keys":[
			{"kid":"k1","kty":"RSA","alg":"RS256","use":"sig",
			 "n":"8Pr1nbtmfj-P4Y4pZRlam3YbQQ3dOC9z2_sQ12B_pxGhkEIWSva1HffxN7AjSxrL6NMGOkPLHRS9CMdY2FUVzdhCBmzipFLmBJ1YPseJjLxpMMayFM6TbLAJtG25RKlNZWGQotPUDdLu1jl-2PrPa-4zjQwZgHet2s226xv1opYG6nShMv3Tjwqq3FjxckIa3bEnvPIG9dQ4DIEKCGP5Q-jRX7wc5gyw3GhzbJmizIAhVmB77XPFfsNuWcqKDbC_zUcFp0rjkDpEg5aRvJnTVl6xRRZ5_NUpkzkUqtQ9oInJCrPZsaIw26u1DUScQGtHC4VYjTdNDiqlMx8tC3u6Vw",
			 "e":"AQAB"}
		]}`))
	}))
	defer srv.Close()

	v := NewVerifier(srv.URL, "samplier")
	if err := v.refreshKeys(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/protocol/openid-connect/certs" {
		t.Fatalf("unexpected JWKS path: %q", gotPath)
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	if _, ok := v.keys["k1"]; !ok {
		t.Fatal("expected key k1 to be cached")
	}
}

func TestRefreshKeysServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	v := NewVerifier(srv.URL, "")
	if err := v.refreshKeys(context.Background()); err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestRefreshKeysNoUsableKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"keys":[{"kid":"ec1","kty":"EC"}]}`))
	}))
	defer srv.Close()

	v := NewVerifier(srv.URL, "")
	if err := v.refreshKeys(context.Background()); err == nil {
		t.Fatal("expected error when no RSA keys present")
	}
}

func TestRefreshKeysNoIssuer(t *testing.T) {
	v := NewVerifier("", "")
	if err := v.refreshKeys(context.Background()); err == nil {
		t.Fatal("expected error with empty issuer")
	}
}

func TestHasRole(t *testing.T) {
	c := &Claims{}
	c.RealmAccess.Roles = []string{"viewer", "editor"}
	if !HasRole(c, "editor") {
		t.Fatal("expected editor role")
	}
	if HasRole(c, "admin") {
		t.Fatal("did not expect admin role")
	}
	if HasRole(nil, "editor") {
		t.Fatal("nil claims must not have roles")
	}
}

func TestClaimsContext(t *testing.T) {
	c := &Claims{Subject: "user-1"}
	ctx := WithClaims(context.Background(), c)
	got, ok := ClaimsFrom(ctx)
	if !ok || got.Subject != "user-1" {
		t.Fatalf("unexpected claims: %v %v", got, ok)
	}
	if _, ok := ClaimsFrom(context.Background()); ok {
		t.Fatal("expected no claims in bare context")
	}
}

func TestIssuerNormalization(t *testing.T) {
	v := NewVerifier("https://example.com/realms/x/", "")
	if v.Issuer() != "https://example.com/realms/x" {
		t.Fatalf("got %q", v.Issuer())
	}
}
