package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"database/sql"
)

type Session struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

func randomToken() (string, error) {
	b := make([]byte, 32)

	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func CreateSession(
	db *sql.DB,
	userID int64,
) (*Session, error) {

	token, err := randomToken()
	if err != nil {
		return nil, err
	}

	csrf, err := randomToken()
	if err != nil {
		return nil, err
	}

	expires := time.Now().UTC().Add(1 * time.Hour)

	_, err = db.Exec(`
		INSERT INTO sessions
		(user_id, token_hash, csrf_token, expires_at)
		VALUES (?, ?, ?, ?)
	`,
		userID,
		tokenHash(token),
		csrf,
		expires,
	)

	if err != nil {
		return nil, err
	}

	return &Session{
		Token:     token,
		CSRFToken: csrf,
		ExpiresAt: expires,
	}, nil
}

func DeleteSession(db *sql.DB, token string) error {
	_, err := db.Exec(
		"DELETE FROM sessions WHERE token_hash = ?",
		tokenHash(token),
	)

	return err
}
