package files

import (
	"os"
	"strconv"
)

// S3ConfigFromEnv reads S3_* (see .env.example). Credentials come from the environment, injected
// from OpenBao in deployed environments (CLAUDE.md rule 14).
func S3ConfigFromEnv() S3Config {
	return S3Config{
		Endpoint:  envOr("S3_ENDPOINT", "localhost:9000"),
		AccessKey: os.Getenv("S3_ACCESS_KEY"),
		SecretKey: os.Getenv("S3_SECRET_KEY"),
		Bucket:    envOr("S3_BUCKET", "pdpa-files"),
		UseTLS:    envBool("S3_USE_TLS", false),
		Region:    envOr("S3_REGION", "us-east-1"),
		SSE:       envBool("S3_SSE", true),
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envBool(k string, d bool) bool {
	if b, err := strconv.ParseBool(os.Getenv(k)); err == nil {
		return b
	}
	return d
}
