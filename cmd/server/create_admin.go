package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func createAdmin(db *sql.DB) error {
	var count int

	if err := db.QueryRow(
		"SELECT COUNT(*) FROM users WHERE role = 'admin'",
	).Scan(&count); err != nil {
		return err
	}

	if count > 0 {
		return fmt.Errorf("an administrator already exists")
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Admin username: ")

	username, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	username = strings.TrimSpace(username)

	if len(username) < 3 || len(username) > 64 {
		return fmt.Errorf("invalid username length")
	}

	fmt.Print("Admin password: ")

	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()

	if err != nil {
		return err
	}

	if len(password) < 8 || len(password) > 72 {
		return fmt.Errorf("password must be between 8 and 72 bytes")
	}

	hash, err := bcrypt.GenerateFromPassword(
		password,
		bcrypt.DefaultCost,
	)

	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT INTO users
		(username, password_hash, role)
		VALUES (?, ?, 'admin')
	`, username, string(hash))

	if err != nil {
		return err
	}

	fmt.Println("Administrator created successfully.")

	return nil
}
