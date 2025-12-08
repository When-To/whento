// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package jwt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token has expired")
	ErrInvalidSignature = errors.New("invalid token signature")
)

// Claims represents JWT claims
type Claims struct {
	jwt.RegisteredClaims
	UserID     string `json:"user_id"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	MFAPending bool   `json:"mfa_pending,omitempty"`
}

// Config holds JWT configuration
type Config struct {
	PrivateKeyPath string
	PublicKeyPath  string
	AccessExpiry   time.Duration
	RefreshExpiry  time.Duration
	Issuer         string
}

// Manager handles JWT operations
type Manager struct {
	privateKey    *rsa.PrivateKey
	publicKey     *rsa.PublicKey
	accessExpiry  time.Duration
	refreshExpiry time.Duration
	issuer        string
}

// NewManager creates a new JWT manager
func NewManager(cfg *Config) (*Manager, error) {
	var privateKey *rsa.PrivateKey
	var err error

	// Only load private key if path is provided (for token generation)
	if cfg.PrivateKeyPath != "" {
		privateKey, err = loadPrivateKey(cfg.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load private key: %w", err)
		}
	}

	publicKey, err := loadPublicKey(cfg.PublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key: %w", err)
	}

	return &Manager{
		privateKey:    privateKey,
		publicKey:     publicKey,
		accessExpiry:  cfg.AccessExpiry,
		refreshExpiry: cfg.RefreshExpiry,
		issuer:        cfg.Issuer,
	}, nil
}

// GenerateAccessToken generates a new access token
func (m *Manager) GenerateAccessToken(userID, email, role string) (string, error) {
	if m.privateKey == nil {
		return "", errors.New("private key not loaded - cannot generate tokens")
	}

	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			Subject:   userID,
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessExpiry)),
		},
		UserID: userID,
		Email:  email,
		Role:   role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(m.privateKey)
}

// GenerateRefreshToken generates a new refresh token
func (m *Manager) GenerateRefreshToken(userID string) (string, time.Time, error) {
	if m.privateKey == nil {
		return "", time.Time{}, errors.New("private key not loaded - cannot generate tokens")
	}

	now := time.Now()
	expiresAt := now.Add(m.refreshExpiry)

	claims := jwt.RegisteredClaims{
		ID:        uuid.New().String(),
		Subject:   userID,
		Issuer:    m.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(m.privateKey)
	return tokenString, expiresAt, err
}

// ValidateAccessToken validates an access token and returns claims
func (m *Manager) ValidateAccessToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.publicKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ValidateRefreshToken validates a refresh token and returns the user ID
func (m *Manager) ValidateRefreshToken(tokenString string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.publicKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", ErrExpiredToken
		}
		return "", fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return "", ErrInvalidToken
	}

	return claims.Subject, nil
}

// GetPublicKey returns the public key for external verification
func (m *Manager) GetPublicKey() *rsa.PublicKey {
	return m.publicKey
}

// GenerateCustomToken generates a token with custom claims (for MFA temp tokens)
func (m *Manager) GenerateCustomToken(customClaims map[string]interface{}) (string, error) {
	if m.privateKey == nil {
		return "", errors.New("private key not loaded - cannot generate tokens")
	}

	// Create map claims for flexibility
	mapClaims := jwt.MapClaims{}
	for k, v := range customClaims {
		mapClaims[k] = v
	}

	// Add standard claims if not provided
	if _, ok := mapClaims["iss"]; !ok {
		mapClaims["iss"] = m.issuer
	}
	if _, ok := mapClaims["iat"]; !ok {
		mapClaims["iat"] = time.Now().Unix()
	}
	if _, ok := mapClaims["jti"]; !ok {
		mapClaims["jti"] = uuid.New().String()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, mapClaims)
	return token.SignedString(m.privateKey)
}

// ValidateCustomToken validates a custom token and returns the claims
func (m *Manager) ValidateCustomToken(tokenString string) (map[string]interface{}, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.publicKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1 format
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("key is not an RSA private key")
	}

	return rsaKey, nil
}

func loadPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("key is not an RSA public key")
	}

	return rsaKey, nil
}
