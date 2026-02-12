package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"parenta/internal/api"
	"parenta/internal/config"
	"parenta/internal/services"
	"parenta/internal/storage"
)

var Version = "1.0.0"

func main() {
	// Parse command line flags
	configPath := flag.String("config", "configs/parenta.json", "Path to config file")
	webDir := flag.String("web", "web", "Path to web static files directory")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Parenta v%s\n", Version)
		os.Exit(0)
	}

	log.Printf("Starting Parenta v%s", Version)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("Loaded config from %s", *configPath)

	// Ensure data directory exists
	dataDir := cfg.Storage.DataDir
	if !filepath.IsAbs(dataDir) {
		// Make relative to executable directory
		execDir, _ := os.Getwd()
		dataDir = filepath.Join(execDir, dataDir)
	}

	// Initialize storage
	store, err := storage.New(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	log.Printf("Storage initialized at %s", dataDir)

	// Initialize services
	ndsctl := services.NewNDSCtl(cfg.OpenNDS.NDSCtlPath)
	dnsmasq := services.NewDnsmasqService(store, cfg.Dnsmasq.ConfDir, cfg.Dnsmasq.RestartCmd)
	authSvc := services.NewAuthService(store, cfg.Session.JWTSecret, cfg.Session.JWTExpiryHours)

	// Initialize default admin user
	if err := authSvc.InitializeAdmin(
		cfg.Defaults.AdminUsername,
		cfg.Defaults.AdminPassword,
		cfg.Defaults.ForcePasswordChange,
	); err != nil {
		log.Printf("Warning: Failed to initialize admin: %v", err)
	}

	// Start session ticker
	ticker := services.NewSessionTicker(store, ndsctl, cfg.Session.TickIntervalSeconds)
	ticker.Start()
	log.Printf("Session ticker started (interval: %ds)", cfg.Session.TickIntervalSeconds)

	// Generate initial dnsmasq configs
	if err := dnsmasq.RegenerateConfigs(); err != nil {
		log.Printf("Warning: Failed to generate dnsmasq configs: %v", err)
	}

	// Setup HTTP router
	router := api.NewRouter(cfg, store, ndsctl, dnsmasq, authSvc)
	handler := router.Setup(*webDir)

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// Start server in goroutine
	go func() {
		log.Printf("HTTP server listening on %s", addr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")

	// Stop ticker
	ticker.Stop()

	// Close server
	server.Close()

	log.Println("Parenta stopped")
}
