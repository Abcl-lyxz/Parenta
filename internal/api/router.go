package api

import (
	"net/http"

	"parenta/internal/api/handlers"
	"parenta/internal/api/middleware"
	"parenta/internal/config"
	"parenta/internal/services"
	"parenta/internal/storage"
)

// Router sets up all HTTP routes
type Router struct {
	mux        *http.ServeMux
	auth       *middleware.AuthMiddleware
	storage    *storage.Storage
	config     *config.Config
	ndsctl     *services.NDSCtl
	dnsmasq    *services.DnsmasqService
	authSvc    *services.AuthService
}

// NewRouter creates a new Router
func NewRouter(
	cfg *config.Config,
	store *storage.Storage,
	ndsctl *services.NDSCtl,
	dnsmasq *services.DnsmasqService,
	authSvc *services.AuthService,
) *Router {
	return &Router{
		mux:     http.NewServeMux(),
		auth:    middleware.NewAuthMiddleware(cfg.Session.JWTSecret),
		storage: store,
		config:  cfg,
		ndsctl:  ndsctl,
		dnsmasq: dnsmasq,
		authSvc: authSvc,
	}
}

// Setup registers all routes
func (r *Router) Setup(webDir string) http.Handler {
	// Create handlers
	authHandler := handlers.NewAuthHandler(r.storage, r.authSvc, r.auth, r.config)
	fasHandler := handlers.NewFASHandler(r.storage, r.ndsctl, r.authSvc, r.config)
	childrenHandler := handlers.NewChildrenHandler(r.storage, r.authSvc)
	sessionsHandler := handlers.NewSessionsHandler(r.storage, r.ndsctl)
	schedulesHandler := handlers.NewSchedulesHandler(r.storage)
	filtersHandler := handlers.NewFiltersHandler(r.storage, r.dnsmasq)
	systemHandler := handlers.NewSystemHandler(r.storage, r.ndsctl, r.dnsmasq, r.config)

	// FAS routes (no auth required - these are captive portal entry points)
	r.mux.HandleFunc("/fas/", fasHandler.HandleFAS)
	r.mux.HandleFunc("/fas/auth", fasHandler.HandleAuth)
	r.mux.HandleFunc("/fas/status", fasHandler.HandleStatus)

	// Portal page
	r.mux.HandleFunc("/portal", fasHandler.HandlePortal)

	// Auth routes
	r.mux.HandleFunc("/api/auth/login", authHandler.HandleLogin)
	r.mux.HandleFunc("/api/auth/logout", r.requireAuth(authHandler.HandleLogout))
	r.mux.HandleFunc("/api/auth/me", r.requireAuth(authHandler.HandleMe))
	r.mux.HandleFunc("/api/auth/password", r.requireAuth(authHandler.HandleChangePassword))

	// Admin management routes
	r.mux.HandleFunc("/api/admins", r.requireAuth(authHandler.HandleListAdmins))
	r.mux.HandleFunc("/api/admins/", r.requireAuth(authHandler.HandleAdmin))

	// Children routes
	r.mux.HandleFunc("/api/children", r.requireAuth(childrenHandler.Handle))
	r.mux.HandleFunc("/api/children/", r.requireAuth(childrenHandler.HandleByID))

	// Sessions routes
	r.mux.HandleFunc("/api/sessions", r.requireAuth(sessionsHandler.Handle))
	r.mux.HandleFunc("/api/sessions/", r.requireAuth(sessionsHandler.HandleByID))

	// Schedules routes
	r.mux.HandleFunc("/api/schedules", r.requireAuth(schedulesHandler.Handle))
	r.mux.HandleFunc("/api/schedules/", r.requireAuth(schedulesHandler.HandleByID))

	// Filters routes
	r.mux.HandleFunc("/api/filters", r.requireAuth(filtersHandler.Handle))
	r.mux.HandleFunc("/api/filters/", r.requireAuth(filtersHandler.HandleByID))
	r.mux.HandleFunc("/api/filters/reload", r.requireAuth(filtersHandler.HandleReload))

	// System routes
	r.mux.HandleFunc("/api/system/status", r.requireAuth(systemHandler.HandleStatus))
	r.mux.HandleFunc("/api/system/restart", r.requireAuth(systemHandler.HandleRestart))
	r.mux.HandleFunc("/api/system/health", r.requireAuth(systemHandler.HandleHealth))
	r.mux.HandleFunc("/api/system/command", r.requireAuth(systemHandler.HandleCommand))
	r.mux.HandleFunc("/api/system/logs", r.requireAuth(systemHandler.HandleLogs))
	r.mux.HandleFunc("/api/system/dashboard", r.requireAuth(systemHandler.HandleDashboard))

	// Static files (SPA)
	fileServer := http.FileServer(http.Dir(webDir))
	r.mux.Handle("/", fileServer)

	// Add CORS headers
	return r.corsMiddleware(r.mux)
}

// requireAuth wraps a handler with authentication
func (r *Router) requireAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.auth.RequireAuth(http.HandlerFunc(handler)).ServeHTTP(w, req)
	}
}

// corsMiddleware adds CORS headers
func (r *Router) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if req.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, req)
	})
}
