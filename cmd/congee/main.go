package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/storage/sqlite"
)

func main() {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "./config.json"
	}
	cfg, err := config.Load(path)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()
	if cfg.Database.Type != "" && cfg.Database.Type != "sqlite" {
		log.Fatalf("unsupported database.type %q (phase 3: sqlite only)", cfg.Database.Type)
	}
	st, err := sqlite.Open(ctx, cfg.Database.DSN, nil)
	if err != nil {
		log.Fatalf("sqlite: %v", err)
	}
	defer st.Close()
	fmt.Println("congee: config ok, sqlite store open")
}
