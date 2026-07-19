package config

import (
	"fmt"
	"os"
)

type Config struct {
	APIPort   string
	RedisAddr string
}

func Load() Config {
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")

	return Config{
		APIPort:   getEnv("API_PORT", "8080"),
		RedisAddr: fmt.Sprintf("%s:%s", redisHost, redisPort),
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
