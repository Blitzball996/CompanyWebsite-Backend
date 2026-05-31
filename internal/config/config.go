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
	LicenseKeyB64  string // Ed25519 私钥（base64，可空=用文件）
	LicenseKeyPath string // 私钥文件路径（首次自动生成）
	WebhookSecret  string // 支付平台 webhook 签名密钥
	SMTPHost       string // 邮件服务器（发序列号用）
	SMTPPort       string
	SMTPUser       string
	SMTPPass       string
	SMTPFrom       string // 发件人地址
	AppBaseURL     string // 邮件链接用的站点根 URL（验证/找回）
	CookieSecure   bool   // 会话 cookie 是否加 Secure（生产 https=true）
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
		LicenseKeyB64:  env("LICENSE_PRIVATE_KEY", ""),
		LicenseKeyPath: env("LICENSE_KEY_PATH", "internal/license/data/ed25519.key"),
		WebhookSecret:  env("WEBHOOK_SECRET", ""),
		SMTPHost:       env("SMTP_HOST", ""),
		SMTPPort:       env("SMTP_PORT", "587"),
		SMTPUser:       env("SMTP_USER", ""),
		SMTPPass:       env("SMTP_PASS", ""),
		SMTPFrom:       env("SMTP_FROM", ""),
		AppBaseURL:     env("APP_BASE_URL", ""),
		CookieSecure:   env("COOKIE_SECURE", "false") == "true",
	}
}
