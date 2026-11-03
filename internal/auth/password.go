package auth

import "golang.org/x/crypto/bcrypt"

func HashPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", ErrWeakPassword
	}

	hash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)

	return string(hash), err
}

func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword(
		[]byte(hash),
		[]byte(password),
	) == nil
}
