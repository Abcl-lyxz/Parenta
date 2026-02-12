package services

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"parenta/internal/models"
	"parenta/internal/storage"
)

// DnsmasqService manages dnsmasq filter configurations
type DnsmasqService struct {
	storage    *storage.Storage
	confDir    string
	restartCmd string
}

// NewDnsmasqService creates a new DnsmasqService
func NewDnsmasqService(store *storage.Storage, confDir, restartCmd string) *DnsmasqService {
	return &DnsmasqService{
		storage:    store,
		confDir:    confDir,
		restartCmd: restartCmd,
	}
}

// RegenerateConfigs rebuilds all dnsmasq filter config files
func (d *DnsmasqService) RegenerateConfigs() error {
	// Ensure config directory exists
	if err := os.MkdirAll(d.confDir, 0755); err != nil {
		return fmt.Errorf("create conf dir: %w", err)
	}

	// Generate blocklist
	blacklist := d.storage.ListFilters(models.RuleTypeBlacklist)
	if err := d.writeBlocklist(blacklist); err != nil {
		return fmt.Errorf("write blocklist: %w", err)
	}

	// Generate whitelist
	whitelist := d.storage.ListFilters(models.RuleTypeWhitelist)
	if err := d.writeWhitelist(whitelist); err != nil {
		return fmt.Errorf("write whitelist: %w", err)
	}

	return nil
}

// Reload restarts dnsmasq to apply new configuration
func (d *DnsmasqService) Reload() error {
	parts := strings.Fields(d.restartCmd)
	if len(parts) == 0 {
		return fmt.Errorf("invalid restart command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("dnsmasq restart: %s", errMsg)
		}
		return fmt.Errorf("dnsmasq restart: %w", err)
	}
	return nil
}

// ApplyAndReload regenerates configs and reloads dnsmasq
func (d *DnsmasqService) ApplyAndReload() error {
	if err := d.RegenerateConfigs(); err != nil {
		return err
	}
	return d.Reload()
}

// writeBlocklist writes the blocklist dnsmasq config
func (d *DnsmasqService) writeBlocklist(rules []*models.FilterRule) error {
	var buf bytes.Buffer
	buf.WriteString("# Parenta Blocklist - Auto-generated\n")
	buf.WriteString("# Do not edit manually - changes will be overwritten\n\n")

	for _, rule := range rules {
		// address=/domain.com/ returns NXDOMAIN for that domain
		domain := strings.TrimPrefix(rule.Domain, "*.")
		fmt.Fprintf(&buf, "address=/%s/\n", domain)
	}

	path := filepath.Join(d.confDir, "parenta-blocklist.conf")
	return d.atomicWrite(path, buf.Bytes())
}

// writeWhitelist writes the whitelist dnsmasq config
func (d *DnsmasqService) writeWhitelist(rules []*models.FilterRule) error {
	var buf bytes.Buffer
	buf.WriteString("# Parenta Whitelist - Auto-generated\n")
	buf.WriteString("# Do not edit manually - changes will be overwritten\n\n")

	// For study mode: forward whitelisted domains to upstream DNS
	for _, rule := range rules {
		domain := strings.TrimPrefix(rule.Domain, "*.")
		// server=/domain.com/8.8.8.8 forwards queries to upstream
		fmt.Fprintf(&buf, "server=/%s/8.8.8.8\n", domain)
	}

	path := filepath.Join(d.confDir, "parenta-whitelist.conf")
	return d.atomicWrite(path, buf.Bytes())
}

// atomicWrite writes data to a file atomically using temp + rename
func (d *DnsmasqService) atomicWrite(path string, data []byte) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// GenerateStudyModeBlock generates config that blocks all DNS except whitelist
func (d *DnsmasqService) GenerateStudyModeBlock() error {
	var buf bytes.Buffer
	buf.WriteString("# Parenta Study Mode - Block all non-whitelisted\n")
	buf.WriteString("# This blocks ALL domains by default\n\n")
	buf.WriteString("address=/#/\n") // NXDOMAIN for all domains

	path := filepath.Join(d.confDir, "parenta-studymode.conf")
	return d.atomicWrite(path, buf.Bytes())
}

// EnableStudyMode activates study mode (block all except whitelist)
func (d *DnsmasqService) EnableStudyMode() error {
	if err := d.GenerateStudyModeBlock(); err != nil {
		return err
	}
	return d.Reload()
}

// DisableStudyMode deactivates study mode
func (d *DnsmasqService) DisableStudyMode() error {
	path := filepath.Join(d.confDir, "parenta-studymode.conf")
	// Remove the file if it exists
	os.Remove(path)
	return d.Reload()
}
