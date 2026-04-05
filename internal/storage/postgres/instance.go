package postgres

import (
	"os"

	"github.com/google/uuid"
)

func localInstanceID() string {
	if s := os.Getenv("CONGEE_INSTANCE_ID"); s != "" {
		return s
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	return host + "-" + uuid.New().String()
}
