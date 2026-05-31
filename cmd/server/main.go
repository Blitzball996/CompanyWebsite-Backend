package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"blitzball-analytics/internal/blog"
	"blitzball-analytics/internal/collect"
	"blitzball-analytics/internal/config"
	"blitzball-analytics/internal/dashboard"
	"blitzball-analytics/internal/db"
	"blitzball-analytics/internal/geo"
	"blitzball-analytics/internal/license"
	"blitzball-analytics/internal/mailer"
	mw "blitzball-analytics/internal/middleware"
	"blitzball-analytics/internal/stats"
	"blitzball-analytics/internal/webhook"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(context.Background(), pool, "internal/db/migrations"); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("migrations applied")

	// Load offline geo database (degrades gracefully to empty if missing).
	if err := geo.Init(cfg.GeoDBPath); err != nil {
		log.Printf("geo: ip2region db not loaded (%v) — geolocation disabled", err)
	} else {
		log.Println("geo: ip2region loaded")
	}

	coll := collect.New(pool, cfg.VisitorSalt, cfg.StoreIP)
	st := stats.New(pool)
	bl := blog.New(pool)

	// License signing key (Ed25519). The printed public key goes into the apps.
	priv, pubB64, err := license.LoadOrCreateKey(cfg.LicenseKeyB64, cfg.LicenseKeyPath)
	if err != nil {
		log.Fatalf("license key: %v", err)
	}
	lic := license.New(pool, priv)
	log.Printf("license: Ed25519 ready — embed this PUBLIC KEY in the apps: %s", pubB64)

	// Mailer + payment webhook (auto-issue license on successful payment).
	mail := mailer.New(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom)
	if mail.Enabled() {
		log.Println("mailer: SMTP configured")
	} else {
		log.Println("mailer: SMTP not configured — keys will be issued but not emailed")
	}
	wh := webhook.New(cfg.WebhookSecret, lic, mail)

	dash, err := dashboard.New(cfg.DashboardUser, cfg.DashboardPass, "internal/dashboard/templates")
	if err != nil {
		log.Fatalf("dashboard: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// Health
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	// Payment webhook (Airwallex). Signature-verified inside the handler; no auth
	// middleware so the provider can reach it. Idempotent issue + email.
	r.Post("/api/webhook/airwallex", wh.Airwallex)

	// Public collect endpoint (CORS + rate limit)
	r.Group(func(r chi.Router) {
		r.Use(mw.CORS(cfg.AllowedOrigins))
		r.Use(mw.RateLimit(120, 60)) // 120/min, burst 60 per IP
		r.Post("/api/collect", coll.Collect)
		r.Options("/api/collect", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
		// Public blog read (the website fetches this to render "latest news")
		r.Get("/api/posts", bl.List)
	})

	// Public license activation (called by the desktop apps; CORS + rate limit)
	r.Group(func(r chi.Router) {
		r.Use(mw.CORS(cfg.AllowedOrigins))
		r.Use(mw.RateLimit(60, 20)) // tighter: 60/min, burst 20 per IP
		r.Post("/api/license/activate", lic.Activate)
		r.Post("/api/license/verify", lic.Verify)
		r.Get("/api/license/pubkey", lic.PublicKey)
	})

	// Serve the analytics.js script for the website to include
	r.Get("/analytics.js", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, req, "web/analytics.js")
	})

	// Protected dashboard + stats API + blog admin (HTTP Basic Auth)
	r.Group(func(r chi.Router) {
		r.Use(dash.BasicAuth)
		r.Get("/dashboard", dash.Page)
		r.Get("/api/stats/summary", st.Summary)
		r.Get("/api/stats/trend", st.Trend)
		r.Get("/api/stats/pages", st.Pages)
		r.Get("/api/stats/referrers", st.Referrers)
		r.Get("/api/stats/devices", st.Devices)
		r.Get("/api/stats/browsers", st.Browsers)
		r.Get("/api/stats/os", st.OS)
		r.Get("/api/stats/langs", st.Langs)
		r.Get("/api/stats/events", st.Events)
		r.Get("/api/stats/regions", st.Regions)
		r.Get("/api/stats/visitors", st.Visitors)
		r.Get("/api/stats/journey", st.VisitorJourney)
		// Blog admin
		r.Get("/api/admin/posts", bl.ListAll)
		r.Post("/api/admin/posts", bl.Create)
		r.Put("/api/admin/posts/{id}", bl.Update)
		r.Delete("/api/admin/posts/{id}", bl.Delete)
		// License admin (generate / list / revoke / reset device binding)
		r.Get("/api/admin/licenses", lic.ListAdmin)
		r.Post("/api/admin/licenses", lic.GenerateAdmin)
		r.Post("/api/admin/licenses/{key}/revoke", lic.RevokeAdmin)
		r.Post("/api/admin/licenses/{key}/reset-device", lic.ResetDeviceAdmin)
	})

	addr := ":" + cfg.Port
	log.Printf("Blitzball Analytics listening on %s  (dashboard: /dashboard)", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
