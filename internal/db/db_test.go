package db

import (
	"context"
	"testing"
	"time"

	"github.com/samplier/samplier/internal/testsupport"
)

func TestConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := Connect(ctx, testsupport.DatabaseURL())
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestConnectInvalidURL(t *testing.T) {
	if _, err := Connect(context.Background(), "not-a-valid-url"); err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestConnectUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Port 1 is not listening.
	if _, err := Connect(ctx, "postgres://user:pass@127.0.0.1:1/none"); err == nil {
		t.Fatal("expected connection error")
	}
}
