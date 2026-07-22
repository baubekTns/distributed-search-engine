package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	APIPort       string
	RedisAddr     string
	OpenSearchURL string
	PostgresDSN   string

	CrawlerUserAgent         string
	CrawlerRequestTimeout    time.Duration
	CrawlerRequestDelay      time.Duration
	CrawlerMaxResponseBytes  int64
	CrawlerMaxRedirects      int
	CrawlerMaxLinksPerPage   int
	CrawlerMaxDepth          int
	CrawlerMaxPagesPerDomain int
	CrawlerMaxRetries        int
	CrawlerWorkerCount       int
	CrawlerHeartbeatInterval time.Duration
}

func Load() Config {
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")

	postgresHost := getEnv("POSTGRES_HOST", "localhost")
	postgresPort := getEnv("POSTGRES_PORT", "5432")
	postgresDatabase := getEnv("POSTGRES_DB", "search_engine")
	postgresUser := getEnv("POSTGRES_USER", "search_user")
	postgresPassword := getEnv(
		"POSTGRES_PASSWORD",
		"search_password",
	)

	return Config{
		APIPort:   getEnv("API_PORT", "8080"),
		RedisAddr: fmt.Sprintf("%s:%s", redisHost, redisPort),
		OpenSearchURL: getEnv(
			"OPENSEARCH_URL",
			"http://localhost:9200",
		),
		PostgresDSN: fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s?sslmode=disable",
			postgresUser,
			postgresPassword,
			postgresHost,
			postgresPort,
			postgresDatabase,
		),

		CrawlerUserAgent: getEnv(
			"CRAWLER_USER_AGENT",
			"StudentSearchBot/1.0",
		),
		CrawlerRequestTimeout: time.Duration(
			getEnvInt("CRAWLER_REQUEST_TIMEOUT_SECONDS", 10),
		) * time.Second,
		CrawlerRequestDelay: time.Duration(
			getEnvInt("CRAWLER_REQUEST_DELAY_SECONDS", 2),
		) * time.Second,
		CrawlerMaxResponseBytes: int64(
			getEnvInt(
				"CRAWLER_MAX_RESPONSE_BYTES",
				5*1024*1024,
			),
		),
		CrawlerMaxRedirects: getEnvInt(
			"CRAWLER_MAX_REDIRECTS",
			3,
		),
		CrawlerMaxLinksPerPage: getEnvInt(
			"CRAWLER_MAX_LINKS_PER_PAGE",
			100,
		),
		CrawlerMaxDepth: getEnvInt(
			"CRAWLER_MAX_DEPTH",
			3,
		),
		CrawlerMaxPagesPerDomain: getEnvInt(
			"CRAWLER_MAX_PAGES_PER_DOMAIN",
			500,
		),
		CrawlerMaxRetries: getEnvInt(
			"CRAWLER_MAX_RETRIES",
			2,
		),
		CrawlerWorkerCount: getEnvInt(
			"CRAWLER_WORKER_COUNT",
			4,
		),
		CrawlerHeartbeatInterval: time.Duration(
			getEnvInt(
				"CRAWLER_HEARTBEAT_INTERVAL_SECONDS",
				10,
			),
		) * time.Second,
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}

	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}