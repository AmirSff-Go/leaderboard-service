package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenGenerator struct {
	secret string
}

func NewTokenGenerator(secret string) *TokenGenerator {
	return &TokenGenerator{secret: secret}
}

func (tg *TokenGenerator) GenerateToken(gameID string, tokenVersion int) (string, error) {
	claims := jwt.MapClaims{
		"game_id":       gameID,
		"token_version": tokenVersion,
		"iat":           time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(tg.secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func (tg *TokenGenerator) ValidateToken(tokenString string) (map[string]interface{}, error) {
	// Parse token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(tg.secret), nil
	})
	if err != nil {
		return nil, err
	}

	// Check if token is valid
	if !token.Valid {
		return nil, jwt.ErrSignatureInvalid
	}

	// Verify signature
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, jwt.ErrSignatureInvalid
	}

	// Step 3: Check claims.token_version == game.TokenVersion
	// Step 4: If mismatch → return error "token revoked"
	// Step 5: Return claims if valid
	return claims, nil
}
