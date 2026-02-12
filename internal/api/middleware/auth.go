package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const (
	UserContextKey contextKey = "user"
)

// JWTClaims represents the JWT payload
type JWTClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	Exp      int64  `json:"exp"`
}

// AuthMiddleware provides JWT authentication
type AuthMiddleware struct {
	secret []byte
}

// NewAuthMiddleware creates a new AuthMiddleware
func NewAuthMiddleware(secret string) *AuthMiddleware {
	return &AuthMiddleware{
		secret: []byte(secret),
	}
}

// RequireAuth middleware that requires valid JWT
func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, `{"error":"invalid authorization header"}`, http.StatusUnauthorized)
			return
		}

		claims, err := m.ValidateToken(parts[1])
		if err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		// Add claims to context
		ctx := context.WithValue(r.Context(), UserContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GenerateToken creates a new JWT token
func (m *AuthMiddleware) GenerateToken(userID, username string, isAdmin bool, expiryHours int) (string, error) {
	claims := JWTClaims{
		UserID:   userID,
		Username: username,
		IsAdmin:  isAdmin,
		Exp:      time.Now().Add(time.Duration(expiryHours) * time.Hour).Unix(),
	}

	// Simple JWT: header.payload.signature
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	payloadBytes, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	signatureInput := header + "." + payload
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(signatureInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signatureInput + "." + signature, nil
}

// ValidateToken verifies and parses a JWT token
func (m *AuthMiddleware) ValidateToken(token string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, http.ErrNoCookie
	}

	// Verify signature
	signatureInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(signatureInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, http.ErrNoCookie
	}

	// Decode payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, err
	}

	// Check expiry
	if claims.Exp < time.Now().Unix() {
		return nil, http.ErrNoCookie
	}

	return &claims, nil
}

// GetClaims extracts claims from request context
func GetClaims(r *http.Request) *JWTClaims {
	claims, ok := r.Context().Value(UserContextKey).(*JWTClaims)
	if !ok {
		return nil
	}
	return claims
}
