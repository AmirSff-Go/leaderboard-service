package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// makeToken builds a raw JWT with arbitrary claims using the given method and secret.
func makeToken(t *testing.T, method jwt.SigningMethod, claims jwt.Claims, secret string) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("makeToken: %v", err)
	}
	return s
}

func gameClaims(gameID string, version int) *GameTokenClaims {
	return &GameTokenClaims{
		GameID:           gameID,
		TokenVersion:     version,
		RegisteredClaims: jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(time.Now())},
	}
}

// ---- GenerateToken ----------------------------------------------------------------

func TestGenerateToken(t *testing.T) {
	tests := []struct {
		name         string
		secret       string
		gameID       string
		tokenVersion int
		wantErr      error
	}{
		{"success", "s3cr3t", "game-1", 1, nil},
		{"multiple token versions", "s3cr3t", "game-1", 42, nil},
		{"empty secret", "", "game-1", 1, ErrInvalidJWTSecret},
		{"empty gameID", "s3cr3t", "", 1, ErrMissingGameID},
		{"zero tokenVersion", "s3cr3t", "game-1", 0, ErrInvalidTokenVer},
		{"negative tokenVersion", "s3cr3t", "game-1", -3, ErrInvalidTokenVer},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tg := NewTokenGenerator(tc.secret)
			tok, err := tg.GenerateToken(tc.gameID, tc.tokenVersion)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want error %v, got %v", tc.wantErr, err)
				}
				if tok != "" {
					t.Fatal("expected empty token on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok == "" {
				t.Fatal("expected non-empty token string")
			}
		})
	}
}

// ---- ParseGameToken ---------------------------------------------------------------

func TestParseGameToken_RoundTrip(t *testing.T) {
	tg := NewTokenGenerator("round-trip-secret")
	tokenStr, err := tg.GenerateToken("game-xyz", 7)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := tg.ParseGameToken(tokenStr)
	if err != nil {
		t.Fatalf("ParseGameToken: %v", err)
	}
	if claims.GameID != "game-xyz" {
		t.Errorf("GameID: want %q, got %q", "game-xyz", claims.GameID)
	}
	if claims.TokenVersion != 7 {
		t.Errorf("TokenVersion: want 7, got %d", claims.TokenVersion)
	}
}

func TestParseGameToken_EmptySecret(t *testing.T) {
	tg := NewTokenGenerator("")
	_, err := tg.ParseGameToken("any.token.value")
	if !errors.Is(err, ErrInvalidJWTSecret) {
		t.Fatalf("want ErrInvalidJWTSecret, got %v", err)
	}
}

func TestParseGameToken_EmptyTokenString(t *testing.T) {
	tg := NewTokenGenerator("s3cr3t")
	_, err := tg.ParseGameToken("")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}

func TestParseGameToken_GarbageString(t *testing.T) {
	tg := NewTokenGenerator("s3cr3t")
	_, err := tg.ParseGameToken("not-a-jwt-at-all")
	if err == nil {
		t.Fatal("expected error for garbage token, got nil")
	}
}

func TestParseGameToken_WrongSignature(t *testing.T) {
	tg1 := NewTokenGenerator("secret-A")
	tg2 := NewTokenGenerator("secret-B")

	tokenStr, err := tg1.GenerateToken("game-1", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	_, err = tg2.ParseGameToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for wrong signature, got nil")
	}
}

func TestParseGameToken_WrongAlgorithm(t *testing.T) {
	const secret = "s3cr3t"
	tg := NewTokenGenerator(secret)

	// Build a token with HS512 instead of the required HS256.
	hs512Token := makeToken(t, jwt.SigningMethodHS512, gameClaims("game-1", 1), secret)

	_, err := tg.ParseGameToken(hs512Token)
	if err == nil {
		t.Fatal("expected error for HS512 token, got nil")
	}
}

func TestParseGameToken_MissingGameID(t *testing.T) {
	const secret = "s3cr3t"
	tg := NewTokenGenerator(secret)

	tok := makeToken(t, jwt.SigningMethodHS256, gameClaims("", 1), secret)

	_, err := tg.ParseGameToken(tok)
	if !errors.Is(err, ErrMissingGameID) {
		t.Fatalf("want ErrMissingGameID, got %v", err)
	}
}

func TestParseGameToken_LowTokenVersion(t *testing.T) {
	const secret = "s3cr3t"
	tg := NewTokenGenerator(secret)

	tok := makeToken(t, jwt.SigningMethodHS256, gameClaims("game-1", 0), secret)

	_, err := tg.ParseGameToken(tok)
	if !errors.Is(err, ErrInvalidTokenVer) {
		t.Fatalf("want ErrInvalidTokenVer, got %v", err)
	}
}

// ---- ValidateToken ----------------------------------------------------------------

func TestValidateToken_Success(t *testing.T) {
	tg := NewTokenGenerator("val-secret")
	tokenStr, err := tg.GenerateToken("game-abc", 2)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	mc, err := tg.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if mc["game_id"] != "game-abc" {
		t.Errorf("game_id: want %q, got %v", "game-abc", mc["game_id"])
	}
	// token_version is float64 in MapClaims (standard JSON number unmarshaling)
	if mc["token_version"].(float64) != 2 {
		t.Errorf("token_version: want 2, got %v", mc["token_version"])
	}
}

func TestValidateToken_GarbageString(t *testing.T) {
	tg := NewTokenGenerator("val-secret")
	_, err := tg.ValidateToken("not.a.jwt")
	if err == nil {
		t.Fatal("expected error for garbage token, got nil")
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	tg1 := NewTokenGenerator("secret-A")
	tg2 := NewTokenGenerator("secret-B")

	tokenStr, err := tg1.GenerateToken("game-1", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	_, err = tg2.ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for wrong secret, got nil")
	}
}

func TestValidateToken_WrongAlgorithm(t *testing.T) {
	const secret = "val-secret"
	tg := NewTokenGenerator(secret)

	hs512Token := makeToken(t, jwt.SigningMethodHS512, gameClaims("game-1", 1), secret)

	_, err := tg.ValidateToken(hs512Token)
	if err == nil {
		t.Fatal("expected error for HS512 token, got nil")
	}
}
