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
	StoreIP        bool   // 是否存储明文 IP（隐私开关）
	GeoDBPath      string // ip2region xdb 路径
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
		StoreIP:        env("STORE_IP", "true") == "true",
		GeoDBPath:      env("GEO_DB_PATH", "internal/geo/data/ip2region.xdb"),
	}
}
