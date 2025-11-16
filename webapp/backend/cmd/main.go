package main

import (
	"backend/internal/server"
)

func main() {
	srv, dbConn, store, _ := server.NewServer()
	if dbConn != nil {
		defer dbConn.Close()
	}
	if store != nil {
		defer func() { _ = store.Close() }()
	}

	srv.Run()
}
