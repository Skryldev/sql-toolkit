package db

import "os"

func envOrEmpty(key string) string { return os.Getenv(key) }