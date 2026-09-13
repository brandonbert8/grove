package auth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RefreshStore tracks live refresh-token ids for rotation. Consume is
// single-use: redeeming deletes the id, so a replayed refresh token is
// rejected — the theft signal. Implement it on Redis/DB for replicas;
// MemoryRefreshStore covers single-process apps and tests.
type RefreshStore interface {
	// Store records id for subject until expires.
	Store(ctx context.Context, id, subject string, expires time.Time) error
	// Consume deletes id, returning its subject when live. Replays and
	// unknown ids report ok=false.
	Consume(ctx context.Context, id string) (subject string, ok bool)
}

// Rotate redeems a refresh token for a fresh pair: it verifies the
// token, consumes its jti (single-use), issues the next pair carrying
// the same extra claims, and stores the new jti. A reused refresh
// token fails here — treat that as compromise and revoke the subject's
// sessions.
func Rotate(ctx context.Context, svc *Service, store RefreshStore, refreshToken string) (TokenPair, error) {
	var zero TokenPair
	claims, err := svc.VerifyRefresh(refreshToken)
	if err != nil {
		return zero, err
	}
	if _, ok := store.Consume(ctx, claims.ID); !ok {
		return zero, fmt.Errorf("auth: unknown or reused refresh token")
	}
	pair, err := svc.IssuePair(claims.Subject, claims.Extra)
	if err != nil {
		return zero, err
	}
	if err := store.Store(ctx, pair.RefreshID, claims.Subject, time.Unix(pair.RefreshExpiresAt, 0)); err != nil {
		return zero, fmt.Errorf("auth: store refresh token: %w", err)
	}
	return pair, nil
}

// refreshEntry is one tracked refresh token.
type refreshEntry struct {
	subject string
	expires time.Time
}

// MemoryRefreshStore is a process-local RefreshStore.
type MemoryRefreshStore struct {
	mu      sync.Mutex
	entries map[string]refreshEntry
}

// NewMemoryRefreshStore returns an empty store.
func NewMemoryRefreshStore() *MemoryRefreshStore {
	return &MemoryRefreshStore{entries: map[string]refreshEntry{}}
}

// Store records id for subject until expires.
func (s *MemoryRefreshStore) Store(_ context.Context, id, subject string, expires time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = map[string]refreshEntry{}
	}
	s.entries[id] = refreshEntry{subject: subject, expires: expires}
	if len(s.entries) > 1024 {
		now := time.Now()
		for k, e := range s.entries {
			if now.After(e.expires) {
				delete(s.entries, k)
			}
		}
	}
	return nil
}

// Consume deletes id, returning its subject when live and unexpired.
func (s *MemoryRefreshStore) Consume(_ context.Context, id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if ok {
		delete(s.entries, id)
	}
	if !ok || time.Now().After(e.expires) {
		return "", false
	}
	return e.subject, true
}
