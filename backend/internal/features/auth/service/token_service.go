package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/bharatchat/backend/internal/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type AccessTokenClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}
type TokenService struct {
	accessSecret []byte
	accessTTL    time.Duration
	issuer       string
	audience     string
}

func NewTokenService(secret string, ttl time.Duration, identity ...string) *TokenService {
	issuer, audience := "bharatchat-api", "bharatchat-clients"
	if len(identity) > 0 && identity[0] != "" {
		issuer = identity[0]
	}
	if len(identity) > 1 && identity[1] != "" {
		audience = identity[1]
	}
	return &TokenService{accessSecret: []byte(secret), accessTTL: ttl, issuer: issuer, audience: audience}
}
func (s *TokenService) GenerateAccessToken(userID, sessionID string) (string, string, time.Time, error) {
	now := time.Now().UTC()
	jti := uuid.NewString()
	exp := now.Add(s.accessTTL)
	claims := AccessTokenClaims{
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: s.issuer, Subject: userID, Audience: jwt.ClaimStrings{s.audience}, ID: jti,
			IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	raw := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err := raw.SignedString(s.accessSecret)
	return token, jti, exp, err
}
func (s *TokenService) ParseAccessToken(raw string) (*middleware.AccessClaims, error) {
	claims := &AccessTokenClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (interface{}, error) {
		return s.accessSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(s.issuer), jwt.WithAudience(s.audience))
	if err != nil || !token.Valid || claims.ID == "" {
		return nil, errors.New("invalid access token")
	}
	uid, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, errors.New("malformed subject")
	}
	sid, err := uuid.Parse(claims.SessionID)
	if err != nil {
		return nil, errors.New("malformed session")
	}
	return &middleware.AccessClaims{UserID: uid, SessionID: sid, JTI: claims.ID}, nil
}
func (s *TokenService) GenerateRefreshToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	return raw, s.HashRefreshToken(raw), nil
}
func (s *TokenService) HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
