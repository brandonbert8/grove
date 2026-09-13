package auth

import (
	"context"
	"testing"
	"time"
)

func TestPasswordRoundtrip(t *testing.T) {
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "secret" {
		t.Fatal("hash must differ from plaintext")
	}
	if err := ComparePassword(hash, "secret"); err != nil {
		t.Fatalf("correct password must verify: %v", err)
	}
	if err := ComparePassword(hash, "wrong"); err == nil {
		t.Fatal("wrong password must fail")
	}
	if _, err := HashPassword(""); err == nil {
		t.Fatal("empty password must fail")
	}
}

func TestIssuePairAndRotate(t *testing.T) {
	svc := testService()
	store := NewMemoryRefreshStore()
	ctx := context.Background()

	pair, err := svc.IssuePair("user-1", map[string]any{"roles": []any{"admin"}})
	if err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" || pair.RefreshID == "" {
		t.Fatalf("pair incomplete: %+v", pair)
	}
	// Access authorizes; refresh does not.
	if _, err := svc.Verify(pair.AccessToken); err != nil {
		t.Fatalf("access must verify: %v", err)
	}
	if _, err := svc.Verify(pair.RefreshToken); err == nil {
		t.Fatal("refresh token must not verify as access")
	}
	rc, err := svc.VerifyRefresh(pair.RefreshToken)
	if err != nil {
		t.Fatalf("refresh must verify: %v", err)
	}
	if rc.ID != pair.RefreshID || rc.Subject != "user-1" {
		t.Fatalf("refresh claims = %+v", rc)
	}

	// Login stores the refresh id; rotation consumes + reissues.
	if err := store.Store(ctx, pair.RefreshID, "user-1", time.Unix(pair.RefreshExpiresAt, 0)); err != nil {
		t.Fatal(err)
	}
	next, err := Rotate(ctx, svc, store, pair.RefreshToken)
	if err != nil {
		t.Fatalf("rotate must succeed: %v", err)
	}
	if next.RefreshID == pair.RefreshID {
		t.Fatal("rotation must mint a fresh jti")
	}
	if _, err := svc.Verify(next.AccessToken); err != nil {
		t.Fatalf("rotated access must verify: %v", err)
	}
	// Replay of the consumed token is theft signal: reject.
	if _, err := Rotate(ctx, svc, store, pair.RefreshToken); err == nil {
		t.Fatal("replayed refresh token must fail")
	}
	// Unknown ids fail even with valid signature shape.
	if _, ok := store.Consume(ctx, "nope"); ok {
		t.Fatal("unknown id must not consume")
	}
}

func TestRefreshExpiry(t *testing.T) {
	svc := NewService([]byte("test-secret-12345"), "test", WithRefreshTTL(-time.Hour))
	pair, err := svc.IssuePair("user-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyRefresh(pair.RefreshToken); err == nil {
		t.Fatal("expired refresh must fail")
	}
}
