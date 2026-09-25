package main

import (
	"os"
	"strings"
)

// config is read from the environment (.env via `make dev`'s docker compose, see
// deploy/compose/docker-compose.yml and .env.example). Every field has a workable local default so
// `go run ./cmd/api` starts without extra setup beyond a running Postgres and Valkey.
type config struct {
	HTTPAddr           string
	DatabaseURL        string
	ValkeyAddr         string
	OIDCIssuer         string
	OIDCJWKSURL        string
	OpenAPISpecPath    string
	OTelEndpoint       string
	CORSAllowedOrigins []string
}

func loadConfig() config {
	return config{
		HTTPAddr:           env("HTTP_ADDR", ":8080"),
		DatabaseURL:        env("DATABASE_URL", "postgres://pdpa_app:pdpa_app@localhost:5432/pdpa?sslmode=disable"),
		ValkeyAddr:         env("VALKEY_ADDR", "localhost:6379"),
		OIDCIssuer:         env("OIDC_ISSUER", "http://localhost:8081/realms/pdpa"),
		OIDCJWKSURL:        env("OIDC_JWKS_URL", env("OIDC_ISSUER", "http://localhost:8081/realms/pdpa")+"/protocol/openid-connect/certs"),
		OpenAPISpecPath:    env("OPENAPI_SPEC_PATH", "api/openapi/openapi.yaml"),
		OTelEndpoint:       env("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		CORSAllowedOrigins: strings.Split(env("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:3001"), ","),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
