package memoryclient

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func EnsureDatabase(defaultDSN, memoriesDSN string) (*sql.DB, error) {
	admin, err := sql.Open("postgres", defaultDSN)
	if err != nil {
		return nil, fmt.Errorf("open admin db: %w", err)
	}
	defer admin.Close()

	if err := admin.Ping(); err != nil {
		return nil, fmt.Errorf("ping admin db: %w", err)
	}

	var exists bool
	err = admin.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = 'memories')").Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("check memories db: %w", err)
	}
	if !exists {
		if _, err := admin.Exec("CREATE DATABASE memories"); err != nil {
			return nil, fmt.Errorf("create memories db: %w", err)
		}
	}

	db, err := sql.Open("postgres", memoriesDSN)
	if err != nil {
		return nil, fmt.Errorf("open memories db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping memories db: %w", err)
	}
	return db, nil
}
