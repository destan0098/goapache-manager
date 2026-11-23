package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"goapache-manager/internal/api"
	"goapache-manager/internal/database"
)

func main() {
	createAdminFlag := flag.Bool(
		"create-admin",
		false,
		"Create initial administrator",
	)

	flag.Parse()

	dbPath := os.Getenv("GOAPACHE_DB")

	if dbPath == "" {
		dbPath = "/var/lib/goapache/goapache.db"
	}

	db, err := database.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if *createAdminFlag {
		if err := createAdmin(db.Conn); err != nil {
			log.Fatal(err)
		}
		return
	}

	router := api.NewRouter(db.Conn)

	server := &http.Server{
		Addr:              "127.0.0.1:9090",
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Println("GoApache Manager listening on 127.0.0.1:9090")

		if err := server.ListenAndServe(); err != nil &&
			err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)

	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}
}
