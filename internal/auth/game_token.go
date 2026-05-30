package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenGenerator struct {
	secret string
}

func NewTokenGenerator(secret string) *TokenGenerator {
	return &TokenGenerator{secret: secret}
}

// GameTokenClaims is the typed JWT payload we use for game authentication.
// Note: We intentionally do not set ExpirationTime; revocation is handled via token_version.
type GameTokenClaims struct {
	GameID       string `json:"game_id"`
	TokenVersion int    `json:"token_version"`
	jwt.RegisteredClaims
}

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrMissingGameID    = errors.New("game_id is required")
	ErrInvalidTokenVer  = errors.New("token_version must be >= 1")
	ErrInvalidJWTSecret = errors.New("jwt secret is empty")
)

func (tg *TokenGenerator) GenerateToken(gameID string, tokenVersion int) (string, error) {
	if tg.secret == "" {
		return "", ErrInvalidJWTSecret
	}
	if gameID == "" {
		return "", ErrMissingGameID
	}
	if tokenVersion < 1 {
		return "", ErrInvalidTokenVer
	}

	claims := GameTokenClaims{
		GameID:       gameID,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(tg.secret))
}

// ParseGameToken validates the signature and returns typed claims.
func (tg *TokenGenerator) ParseGameToken(tokenString string) (*GameTokenClaims, error) {
	if tg.secret == "" {
		return nil, ErrInvalidJWTSecret
	}
	if tokenString == "" {
		return nil, ErrInvalidToken
	}

	claims := &GameTokenClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		m, ok := token.Method.(*jwt.SigningMethodHMAC)
		if !ok || m != jwt.SigningMethodHS256 {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(tg.secret), nil
	})
	if err != nil {
		return nil, err
	}
	if token == nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	if claims.GameID == "" {
		return nil, ErrMissingGameID
	}
	if claims.TokenVersion < 1 {
		return nil, ErrInvalidTokenVer
	}

	return claims, nil
}

// ValidateToken is a generic validator that keeps backward compatibility with earlier code.
// Prefer ParseGameToken for typed access.
func (tg *TokenGenerator) ValidateToken(tokenString string) (map[string]interface{}, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		m, ok := token.Method.(*jwt.SigningMethodHMAC)
		if !ok || m != jwt.SigningMethodHS256 {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(tg.secret), nil
	})
	if err != nil {
		return nil, err
	}
	if token == nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	mc, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}
	return mc, nil
}
