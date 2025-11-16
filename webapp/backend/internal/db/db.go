package db

import (
	"backend/internal/telemetry"
	"context"
	"fmt"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

func InitDBConnection() (*sqlx.DB, error) {
	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		dbUrl = "user:password@tcp(db:3306)/hiroshimauniv2511-db"
	}
	dsn := fmt.Sprintf("%s?charset=utf8mb4&parseTime=True&loc=UTC", dbUrl)

	driverName := telemetry.WrapSQLDriver("mysql")
	dbConn, err := sqlx.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = dbConn.PingContext(ctx)
	if err != nil {
		dbConn.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Tuned connection pool for higher concurrency. Adjust as needed per environment.
	dbConn.SetMaxOpenConns(100)
	dbConn.SetMaxIdleConns(20)
	dbConn.SetConnMaxLifetime(5 * time.Minute)

	return dbConn, nil
}
