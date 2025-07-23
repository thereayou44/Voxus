package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

type JWTManager struct {
	secretKey     string
	tokenDuration time.Duration
}

func NewJWTManager(secret string, duration time.Duration) *JWTManager {
	return &JWTManager{secretKey: secret, tokenDuration: duration}
}

// Generate создаёт JWT для userID
func (m *JWTManager) Generate(userID string) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.tokenDuration)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(m.secretKey))
}

// Verify парсит и проверяет JWT
func (m *JWTManager) Verify(accessToken string) (*jwt.RegisteredClaims, error) {
	token, err := jwt.ParseWithClaims(accessToken, &jwt.RegisteredClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(m.secretKey), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// Expiry возвращает время истечения токена
func (m *JWTManager) Expiry(accessToken string) (time.Time, error) {
	claims, err := m.Verify(accessToken)
	if err != nil {
		return time.Time{}, err
	}
	return claims.ExpiresAt.Time, nil
}

// ExtractTokenFromHeader извлекает токен из Authorization header
func ExtractTokenFromHeader(r *http.Request) (string, error) {
	hdr := r.Header.Get("Authorization")
	parts := strings.SplitN(hdr, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("invalid Authorization header")
	}
	return parts[1], nil
}
