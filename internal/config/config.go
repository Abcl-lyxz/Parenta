package config

import (
	"encoding/json"
	"os"
)

// Config holds all application configuration
type Config struct {
	Server   ServerConfig   `json:"server"`
	Storage  StorageConfig  `json:"storage"`
	OpenNDS  OpenNDSConfig  `json:"opennds"`
	Dnsmasq  DnsmasqConfig  `json:"dnsmasq"`
	Defaults DefaultsConfig `json:"defaults"`
	Session  SessionConfig  `json:"session"`
}

type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type StorageConfig struct {
	DataDir string `json:"data_dir"`
}

type OpenNDSConfig struct {
	NDSCtlPath string `json:"ndsctl_path"`
	FASKey     string `json:"fas_key"`
	GatewayIP  string `json:"gateway_ip"`
}

type DnsmasqConfig struct {
	ConfDir    string `json:"conf_dir"`
	RestartCmd string `json:"restart_cmd"`
}

type DefaultsConfig struct {
	DailyQuotaMinutes   int    `json:"daily_quota_minutes"`
	AdminUsername       string `json:"admin_username"`
	AdminPassword       string `json:"admin_password"`
	ForcePasswordChange bool   `json:"force_password_change"`
	Timezone            string `json:"timezone"`
}

type SessionConfig struct {
	TickIntervalSeconds int    `json:"tick_interval_seconds"`
	JWTSecret           string `json:"jwt_secret"`
	JWTExpiryHours      int    `json:"jwt_expiry_hours"`
}

// Load reads configuration from a JSON file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set defaults if not specified
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Storage.DataDir == "" {
		cfg.Storage.DataDir = "./data"
	}
	if cfg.Session.TickIntervalSeconds == 0 {
		cfg.Session.TickIntervalSeconds = 30
	}
	if cfg.Session.JWTExpiryHours == 0 {
		cfg.Session.JWTExpiryHours = 24
	}
	if cfg.Defaults.DailyQuotaMinutes == 0 {
		cfg.Defaults.DailyQuotaMinutes = 120
	}

	return &cfg, nil
}
