package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Claims is the JWT payload. Standard fields (sub, exp, iat) are carried
// alongside Extra, which holds application claims (roles, tenant, ...).
type Claims struct {
	// Subject identifies the principal (user id, client id).
	Subject string `json:"sub"`
	// IssuedAt and ExpiresAt are Unix seconds.
	IssuedAt  int64 `json:"iat"`
	ExpiresAt int64 `json:"exp"`
	// Extra carries application claims.
	Extra map[string]any `json:"extra,omitempty"`
}

// Valid reports whether the token has not expired (with 30s leeway).
func (c Claims) Valid() bool {
	return c.Subject != "" && time.Now().Unix() < c.ExpiresAt+30
}

// Service issues and verifies HS256 JWTs. Create one per secret/issuer:
//
//	svc := auth.NewService([]byte(os.Getenv("JWT_SECRET")), "myapp",
//	    auth.WithTTL(time.Hour))
type Service struct {
	secret []byte
	issuer string
	ttl    time.Duration
}

// Option tunes a Service.
type Option func(*Service)

// WithTTL sets token lifetime (default 1 hour). Zero keeps the default;
// negative values are honored (useful for expiry tests).
func WithTTL(d time.Duration) Option {
	return func(s *Service) {
		if d != 0 {
			s.ttl = d
		}
	}
}

// NewService builds a Service. It panics on an empty secret: failing
// closed beats issuing tokens nobody can verify.
func NewService(secret []byte, issuer string, opts ...Option) *Service {
	if len(secret) == 0 {
		panic("auth: JWT secret must not be empty")
	}
	s := &Service{secret: secret, issuer: issuer, ttl: time.Hour}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Sign issues a token for subject carrying extra claims.
func (s *Service) Sign(subject string, extra map[string]any) (string, error) {
	if subject == "" {
		return "", fmt.Errorf("auth: subject must not be empty")
	}
	now := time.Now()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body, err := json.Marshal(Claims{
		Subject:   subject,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(s.ttl).Unix(),
		Extra:     extra,
	})
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
func (s *Service) Verify(token string) (Claims, error) {
	var zero Claims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return zero, fmt.Errorf("auth: malformed token")
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return zero, fmt.Errorf("auth: invalid signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return zero, fmt.Errorf("auth: malformed payload: %w", err)
	}
	var c Claims
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&c); err != nil {
		return zero, fmt.Errorf("auth: malformed claims: %w", err)
	}
	if !c.Valid() {
		return zero, fmt.Errorf("auth: token expired or invalid")
	}
	return c, nil
}

// Issuer returns the configured issuer name.
func (s *Service) Issuer() string { return s.issuer }
