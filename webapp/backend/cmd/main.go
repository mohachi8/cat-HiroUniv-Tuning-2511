package main

import (
	"backend/internal/server"
	"backend/internal/telemetry"
	"context"
	"log"
)

func main() {
	shutdown, err := telemetry.Init(context.Background())
	if err != nil {
		log.Printf("telemetry init failed: %v, continuing without telemetry", err)
	} else {
		defer func() { _ = shutdown(context.Background()) }()
	}

	srv, dbConn, store, err := server.NewServer()
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}
	if dbConn != nil {
		defer dbConn.Close()
	}
	if store != nil {
		defer func() {
			if err := store.Close(); err != nil {
				log.Printf("Failed to close store: %v", err)
			}
		}()
	}

	srv.Run()
}
