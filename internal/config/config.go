// Package config reads the only two values that come from the environment: the
// listen port and the SQLite path. Everything else lives in the database and is
// managed from the admin panel.
package config

import "os"

// Config holds the process-level configuration.
type Config struct {
	Addr   string // listen address, e.g. ":8430" (binds 0.0.0.0)
	DBPath string // SQLite file path
}

// Load reads PORT and DB_PATH from the environment, applying defaults.
func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8430"
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "data/x-rest-api.db"
	}
	return Config{Addr: ":" + port, DBPath: dbPath}
}
