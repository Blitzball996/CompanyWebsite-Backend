package config

import (
	"os"
	"strings"
)

type Config struct {
	Port           string
	DatabaseURL    string
	AllowedOrigins []string
	DashboardUser  string
	DashboardPass  string
	VisitorSalt    string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() Config {
	origins := []string{}
	for _, o := range strings.Split(env("ALLOWED_ORIGINS", "http://localhost:8080"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	return Config{
		Port:           env("PORT", "8090"),
		DatabaseURL:    env("DATABASE_URL", "postgres://analytics:analytics@localhost:5432/analytics?sslmode=disable"),
		AllowedOrigins: origins,
		DashboardUser:  env("DASHBOARD_USER", "admin"),
		DashboardPass:  env("DASHBOARD_PASS", "change-me-please"),
		VisitorSalt:    env("VISITOR_SALT", "please-change-this-random-salt"),
	}
}
