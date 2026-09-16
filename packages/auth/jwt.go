package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Token types carried by Claims.Type.
const (
	// TokenTypeAccess authorizes API calls (Authorization: Bearer).
	TokenTypeAccess = "access"
	// TokenTypeRefresh redeems new pairs via Rotate; never authorizes.
	TokenTypeRefresh = "refresh"
)

// Claims is the JWT payload. Standard fields (sub, exp, iat, jti) are
// carried alongside Extra, which holds application claims (roles,
// tenant, ...).
type Claims struct {
	// Subject identifies the principal (user id, client id).
	Subject string `json:"sub"`
	// IssuedAt and ExpiresAt are Unix seconds.
	IssuedAt  int64 `json:"iat"`
	ExpiresAt int64 `json:"exp"`
	// ID is the token id (jti), set on refresh tokens for rotation.
	ID string `json:"jti,omitempty"`
	// Type is "access", "refresh", or "" (pre-type access tokens).
	Type string `json:"type,omitempty"`
	// Extra carries application claims.
	Extra map[string]any `json:"extra,omitempty"`
}

// Valid reports whether the token carries a subject and has not expired.
// Expiry is exact (no grace): clock skew should be handled by short
// negative-TTL tests, not by accepting expired tokens in production.
func (c Claims) Valid() bool {
	if c.Subject == "" || c.ExpiresAt == 0 {
		return false
	}
	return time.Now().Unix() <= c.ExpiresAt
}

// Service issues and verifies HS256 JWTs. Create one per secret/issuer:
//
//	svc := auth.NewService([]byte(os.Getenv("JWT_SECRET")), "myapp",
//	    auth.WithTTL(time.Hour))
//
// Wire the secret from config and fail fast when absent:
//
//	cfg := config.MustLoad(config.WithRequired("JWT_SECRET"))
//	svc := auth.NewService([]byte(cfg.JWTSecret), cfg.AppName)
type Service struct {
	secret     []byte
	issuer     string
	ttl        time.Duration
	refreshTTL time.Duration
}

// Option tunes a Service.
type Option func(*Service)

// WithTTL sets access-token lifetime (default 1 hour). Zero keeps the
// default; negative values are honored (useful for expiry tests).
func WithTTL(d time.Duration) Option {
	return func(s *Service) {
		if d != 0 {
			s.ttl = d
		}
	}
}

// WithRefreshTTL sets refresh-token lifetime (default 7 days). Zero
// keeps the default; negative values are honored (useful for tests).
func WithRefreshTTL(d time.Duration) Option {
	return func(s *Service) {
		if d != 0 {
			s.refreshTTL = d
		}
	}
}

// NewService builds a Service. It panics on an empty secret: failing
// closed beats issuing tokens nobody can verify. The secret is copied
// so callers cannot mutate it after construction.
func NewService(secret []byte, issuer string, opts ...Option) *Service {
	if len(secret) == 0 {
		panic("auth: JWT secret must not be empty")
	}
	cp := append([]byte(nil), secret...)
	s := &Service{secret: cp, issuer: issuer, ttl: time.Hour, refreshTTL: 7 * 24 * time.Hour}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// cloneExtra deep-copies the top-level claims map so concurrent caller
// mutation cannot race signing.
func cloneExtra(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Sign issues an access token for subject carrying extra claims.
func (s *Service) Sign(subject string, extra map[string]any) (string, error) {
	if subject == "" {
		return "", fmt.Errorf("auth: subject must not be empty")
	}
	now := time.Now()
	return s.signClaims(Claims{
		Subject:   subject,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(s.ttl).Unix(),
		Type:      TokenTypeAccess,
		Extra:     cloneExtra(extra),
	})
}

// TokenPair is one login's tokens: a short-lived access token plus a
// single-use refresh token redeemable via Rotate.
type TokenPair struct {
	// AccessToken authorizes API calls.
	AccessToken string
	// RefreshToken redeems the next pair; store its ID server-side.
	RefreshToken string
	// RefreshID is the refresh token's jti, for the RefreshStore.
	RefreshID string
	// ExpiresAt is the access token expiry (Unix seconds).
	ExpiresAt int64
	// RefreshExpiresAt is the refresh token expiry (Unix seconds).
	RefreshExpiresAt int64
}

// IssuePair signs an access token plus a tracked refresh token sharing
// extra claims (roles survive rotation).
func (s *Service) IssuePair(subject string, extra map[string]any) (TokenPair, error) {
	var zero TokenPair
	access, err := s.Sign(subject, extra)
	if err != nil {
		return zero, err
	}
	refresh, id, err := s.signRefresh(subject, extra)
	if err != nil {
		return zero, err
	}
	now := time.Now()
	return TokenPair{
		AccessToken:      access,
		RefreshToken:     refresh,
		RefreshID:        id,
		ExpiresAt:        now.Add(s.ttl).Unix(),
		RefreshExpiresAt: now.Add(s.refreshTTL).Unix(),
	}, nil
}

// SignRefresh issues a refresh token. Prefer IssuePair, which returns
// the jti needed for server-side tracking.
func (s *Service) SignRefresh(subject string, extra map[string]any) (string, error) {
	tok, _, err := s.signRefresh(subject, extra)
	return tok, err
}

// signRefresh signs a refresh token, returning the token and its jti.
func (s *Service) signRefresh(subject string, extra map[string]any) (string, string, error) {
	if subject == "" {
		return "", "", fmt.Errorf("auth: subject must not be empty")
	}
	id, err := newTokenID()
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	tok, err := s.signClaims(Claims{
		Subject:   subject,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(s.refreshTTL).Unix(),
		ID:        id,
		Type:      TokenTypeRefresh,
		Extra:     cloneExtra(extra),
	})
	if err != nil {
		return "", "", err
	}
	return tok, id, nil
}

// newTokenID mints 128-bit hex for jti.
func newTokenID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("auth: mint token id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// signClaims signs prebuilt claims.
func (s *Service) signClaims(c Claims) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("auth: marshal claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	signing := header + "." + payload
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(signing))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signing + "." + sig, nil
}

// Verify checks signature, shape, and expiry, returning the claims.
// Refresh tokens are rejected: present them to Rotate, never as bearer.
// Pre-type access tokens (no Type claim) keep verifying.
func (s *Service) Verify(token string) (Claims, error) {
	c, err := s.verify(token)
	if err != nil {
		return c, err
	}
	if c.Type != "" && c.Type != TokenTypeAccess {
		return Claims{}, fmt.Errorf("auth: not an access token")
	}
	return c, nil
}

// VerifyRefresh checks a refresh token, returning its claims (including
// the jti for store lookup). Access tokens are rejected here.
func (s *Service) VerifyRefresh(token string) (Claims, error) {
	c, err := s.verify(token)
	if err != nil {
		return c, err
	}
	if c.Type != TokenTypeRefresh {
		return Claims{}, fmt.Errorf("auth: not a refresh token")
	}
	if c.ID == "" {
		return Claims{}, fmt.Errorf("auth: refresh token has no id")
	}
	return c, nil
}

// verify checks signature, shape, and expiry, returning the claims.
func (s *Service) verify(token string) (Claims, error) {
	var zero Claims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return zero, fmt.Errorf("auth: malformed token")
	}
	// Enforce the expected header (no alg confusion): HS256/JWT only.
	hdrRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return zero, fmt.Errorf("auth: malformed header")
	}
	var hdr struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(hdrRaw, &hdr); err != nil {
		return zero, fmt.Errorf("auth: malformed header")
	}
	if hdr.Alg != "HS256" {
		return zero, fmt.Errorf("auth: unexpected signing algorithm")
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return zero, fmt.Errorf("auth: malformed signature")
	}
	if !hmac.Equal(mac.Sum(nil), sig) {
		return zero, fmt.Errorf("auth: invalid signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return zero, fmt.Errorf("auth: malformed payload: %w", err)
	}
	var c Claims
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if err := dec.Decode(&c); err != nil {
		return zero, fmt.Errorf("auth: malformed claims: %w", err)
	}
	// Reject trailing garbage (ndjson smuggling / truncated Tampering).
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return zero, fmt.Errorf("auth: malformed claims")
	}
	if !c.Valid() {
		return zero, fmt.Errorf("auth: token expired or invalid")
	}
	return c, nil
}

// Issuer returns the configured issuer name.
func (s *Service) Issuer() string { return s.issuer }
