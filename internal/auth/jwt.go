package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
)

const (
	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
)

type Claims struct {
	Subject  string `json:"sub"`
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Type     string `json:"type"`
	Expiry   int64  `json:"exp"`
	IssuedAt int64  `json:"iat"`
}

type Identity = domain.Identity

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int64  `json:"expires_in"`
}

type TokenManager struct {
	secret     []byte
	ttl        time.Duration
	refreshTTL time.Duration
}

func NewTokenManager(secret string, ttl, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), ttl: ttl, refreshTTL: refreshTTL}
}

func (m *TokenManager) Generate(ctx context.Context, identity Identity) (string, error) {
	pair, err := m.GeneratePair(ctx, identity)
	if err != nil {
		return "", err
	}
	return pair.AccessToken, nil
}

func (m *TokenManager) GeneratePair(_ context.Context, identity Identity) (*TokenPair, error) {
	access, err := m.generate(identity, tokenTypeAccess, m.ttl)
	if err != nil {
		return nil, err
	}

	refresh := ""
	if m.refreshTTL > 0 {
		refresh, err = m.generate(identity, tokenTypeRefresh, m.refreshTTL)
		if err != nil {
			return nil, err
		}
	}

	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(m.ttl.Seconds()),
	}, nil
}

func (m *TokenManager) Parse(_ context.Context, token string) (*Identity, error) {
	claims, err := m.parseClaims(token)
	if err != nil {
		return nil, err
	}
	if claims.Type != tokenTypeAccess {
		return nil, fmt.Errorf("invalid token type")
	}

	return &Identity{
		UserID:   claims.Subject,
		TenantID: claims.TenantID,
		Email:    claims.Email,
		Role:     claims.Role,
	}, nil
}

func (m *TokenManager) ParseRefresh(_ context.Context, token string) (*Identity, error) {
	claims, err := m.parseClaims(token)
	if err != nil {
		return nil, err
	}
	if claims.Type != tokenTypeRefresh {
		return nil, fmt.Errorf("invalid refresh token type")
	}

	return &Identity{
		UserID:   claims.Subject,
		TenantID: claims.TenantID,
		Email:    claims.Email,
		Role:     claims.Role,
	}, nil
}

func (m *TokenManager) generate(identity Identity, tokenType string, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		Subject:  identity.UserID,
		TenantID: identity.TenantID,
		Email:    identity.Email,
		Role:     identity.Role,
		Type:     tokenType,
		Expiry:   now.Add(ttl).Unix(),
		IssuedAt: now.Unix(),
	}

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	return signToken(header, claims, m.secret)
}

func (m *TokenManager) parseClaims(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}

	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(signingInput))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, fmt.Errorf("invalid token signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}

	if time.Now().UTC().Unix() > claims.Expiry {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

func signToken(header any, claims Claims, secret []byte) (string, error) {
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	headerEncoded := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsEncoded := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := headerEncoded + "." + claimsEncoded

	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + signature, nil
}
