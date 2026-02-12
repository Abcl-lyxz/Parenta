package handlers

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"parenta/internal/config"
	"parenta/internal/services"
	"parenta/internal/storage"
)

// SystemHandler handles system status and control endpoints
type SystemHandler struct {
	storage   *storage.Storage
	ndsctl    *services.NDSCtl
	dnsmasq   *services.DnsmasqService
	config    *config.Config
	startTime time.Time
}

// NewSystemHandler creates a new SystemHandler
func NewSystemHandler(
	store *storage.Storage,
	ndsctl *services.NDSCtl,
	dnsmasq *services.DnsmasqService,
	cfg *config.Config,
) *SystemHandler {
	return &SystemHandler{
		storage:   store,
		ndsctl:    ndsctl,
		dnsmasq:   dnsmasq,
		config:    cfg,
		startTime: time.Now(),
	}
}

// StatusResponse represents system status
type StatusResponse struct {
	Version           string        `json:"version"`
	Uptime            string        `json:"uptime"`
	UptimeSeconds     int64         `json:"uptime_seconds"`
	OpenNDSRunning    bool          `json:"opennds_running"`
	DnsmasqRunning    bool          `json:"dnsmasq_running"`
	ActiveSessions    int           `json:"active_sessions"`
	TotalChildren     int           `json:"total_children"`
	MemoryUsageMB     float64       `json:"memory_usage_mb"`
	GoRoutines        int           `json:"go_routines"`
}

// HandleStatus returns system status
func (h *SystemHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Check if services are running
	openNDSRunning := h.ndsctl.IsRunning()
	dnsmasqRunning := h.checkDnsmasq()

	// Get counts
	sessions := h.storage.ListSessions()
	children := h.storage.ListChildren()

	// Memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := time.Since(h.startTime)

	status := StatusResponse{
		Version:        "1.0.0",
		Uptime:         formatDuration(uptime),
		UptimeSeconds:  int64(uptime.Seconds()),
		OpenNDSRunning: openNDSRunning,
		DnsmasqRunning: dnsmasqRunning,
		ActiveSessions: len(sessions),
		TotalChildren:  len(children),
		MemoryUsageMB:  float64(m.Alloc) / 1024 / 1024,
		GoRoutines:     runtime.NumGoroutine(),
	}

	JSON(w, http.StatusOK, status)
}

// RestartRequest represents restart request
type RestartRequest struct {
	Service string `json:"service"` // "opennds" or "dnsmasq"
}

// HandleRestart restarts a service
func (h *SystemHandler) HandleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req RestartRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var err error
	switch req.Service {
	case "opennds":
		err = exec.Command("/etc/init.d/opennds", "restart").Run()
	case "dnsmasq":
		err = h.dnsmasq.Reload()
	default:
		Error(w, http.StatusBadRequest, "invalid service name")
		return
	}

	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to restart service: "+err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]bool{"success": true})
}

// checkDnsmasq checks if dnsmasq is running
func (h *SystemHandler) checkDnsmasq() bool {
	// Try to execute a simple dnsmasq check
	cmd := exec.Command("pidof", "dnsmasq")
	return cmd.Run() == nil
}

// formatDuration formats a duration as human-readable string
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// ============ Health Check ============

// HealthResponse represents health check response
type HealthResponse struct {
	Status           string `json:"status"`
	OpenNDSRunning   bool   `json:"opennds_running"`
	OpenNDSClients   int    `json:"opennds_clients"`
	GatewayInterface string `json:"gateway_interface"`
	GatewayAddress   string `json:"gateway_address"`
	Errors           []string `json:"errors,omitempty"`
}

// HandleHealth returns health check info
func (h *SystemHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var errors []string
	status := "healthy"

	// Check OpenNDS
	openNDSRunning := h.ndsctl.IsRunning()
	if !openNDSRunning {
		errors = append(errors, "OpenNDS is not running")
		status = "degraded"
	}

	// Get OpenNDS client count
	clients := 0
	if ndsClients, err := h.ndsctl.JSON(); err == nil {
		clients = len(ndsClients)
	}

	// Get gateway info from config
	gatewayInterface := "br-guest"
	gatewayAddress := "192.168.2.1"

	resp := HealthResponse{
		Status:           status,
		OpenNDSRunning:   openNDSRunning,
		OpenNDSClients:   clients,
		GatewayInterface: gatewayInterface,
		GatewayAddress:   gatewayAddress,
		Errors:           errors,
	}

	JSON(w, http.StatusOK, resp)
}

// ============ Command Execution ============

// AllowedCommands defines the whitelist of commands that can be executed
var AllowedCommands = map[string][]string{
	"ifconfig": {},
	"ip":       {"addr", "link", "route"},
	"ps":       {},
	"df":       {"-h"},
	"free":     {"-m"},
	"uptime":   {},
	"ndsctl":   {"status", "json"},
	"logread":  {"-l"},
	"iwinfo":   {},
	"uci":      {"show"},
	"cat":      {"/proc/meminfo", "/proc/loadavg"},
}

// CommandRequest represents command execution request
type CommandRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// CommandResponse represents command execution response
type CommandResponse struct {
	Command  string `json:"command"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

// HandleCommand executes a whitelisted command
func (h *SystemHandler) HandleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req CommandRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Check if command is allowed
	allowedArgs, ok := AllowedCommands[req.Command]
	if !ok {
		Error(w, http.StatusForbidden, "command not allowed: "+req.Command)
		return
	}

	// Validate args if command has restricted args
	if len(allowedArgs) > 0 && len(req.Args) > 0 {
		argAllowed := false
		for _, allowed := range allowedArgs {
			if len(req.Args) > 0 && req.Args[0] == allowed {
				argAllowed = true
				break
			}
		}
		if !argAllowed {
			Error(w, http.StatusForbidden, "arguments not allowed for command: "+req.Command)
			return
		}
	}

	// Execute command with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	// Truncate output if too long
	if len(output) > 50000 {
		output = output[:50000] + "\n... (output truncated)"
	}

	resp := CommandResponse{
		Command:  req.Command + " " + strings.Join(req.Args, " "),
		Output:   output,
		ExitCode: exitCode,
	}
	if err != nil && exitCode == -1 {
		resp.Error = err.Error()
	}

	JSON(w, http.StatusOK, resp)
}

// ============ Log Viewer ============

// LogsResponse represents logs response
type LogsResponse struct {
	Filter string   `json:"filter"`
	Lines  []string `json:"lines"`
	Count  int      `json:"count"`
}

// HandleLogs returns filtered system logs
func (h *SystemHandler) HandleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Get query params
	filter := r.URL.Query().Get("filter") // parenta, opennds, dnsmasq, or empty for all
	linesStr := r.URL.Query().Get("lines")
	lines := 100
	if linesStr != "" {
		if n, err := strconv.Atoi(linesStr); err == nil && n > 0 && n <= 500 {
			lines = n
		}
	}

	// Execute logread with filter
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "logread", "-l", strconv.Itoa(lines))
	output, err := cmd.Output()
	if err != nil {
		// Fallback: try reading from /var/log/messages
		output, err = os.ReadFile("/var/log/messages")
		if err != nil {
			Error(w, http.StatusInternalServerError, "failed to read logs")
			return
		}
	}

	// Filter lines
	var filteredLines []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if filter == "" {
			filteredLines = append(filteredLines, line)
		} else {
			lowerLine := strings.ToLower(line)
			if strings.Contains(lowerLine, strings.ToLower(filter)) {
				filteredLines = append(filteredLines, line)
			}
		}
	}

	// Limit to last N lines
	if len(filteredLines) > lines {
		filteredLines = filteredLines[len(filteredLines)-lines:]
	}

	resp := LogsResponse{
		Filter: filter,
		Lines:  filteredLines,
		Count:  len(filteredLines),
	}

	JSON(w, http.StatusOK, resp)
}

// ============ Dashboard Metrics ============

// DashboardResponse represents enhanced dashboard metrics
type DashboardResponse struct {
	// Existing
	Version        string  `json:"version"`
	Uptime         string  `json:"uptime"`
	UptimeSeconds  int64   `json:"uptime_seconds"`
	OpenNDSRunning bool    `json:"opennds_running"`
	DnsmasqRunning bool    `json:"dnsmasq_running"`
	ActiveSessions int     `json:"active_sessions"`
	TotalChildren  int     `json:"total_children"`

	// New metrics
	MemoryUsedMB    float64 `json:"memory_used_mb"`
	MemoryTotalMB   float64 `json:"memory_total_mb"`
	MemoryPercent   float64 `json:"memory_percent"`
	CPULoad         string  `json:"cpu_load"`
	DiskUsedPercent float64 `json:"disk_used_percent"`
	OpenNDSClients  int     `json:"opennds_clients"`
	LowQuotaAlerts  int     `json:"low_quota_alerts"`
}

// HandleDashboard returns enhanced dashboard metrics
func (h *SystemHandler) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Basic info
	openNDSRunning := h.ndsctl.IsRunning()
	dnsmasqRunning := h.checkDnsmasq()
	sessions := h.storage.ListSessions()
	children := h.storage.ListChildren()
	uptime := time.Since(h.startTime)

	// Memory stats from Go runtime
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// System memory from /proc/meminfo
	memUsed, memTotal := h.getSystemMemory()
	memPercent := 0.0
	if memTotal > 0 {
		memPercent = (memUsed / memTotal) * 100
	}

	// CPU load from /proc/loadavg
	cpuLoad := h.getCPULoad()

	// Disk usage
	diskPercent := h.getDiskUsage()

	// OpenNDS client count
	ndsClients := 0
	if clientList, err := h.ndsctl.JSON(); err == nil {
		ndsClients = len(clientList)
	}

	// Count children with low quota (less than 15 minutes remaining)
	lowQuotaAlerts := 0
	for _, child := range children {
		remaining := child.DailyQuotaMin - child.UsedTodayMin
		if remaining < 15 && remaining >= 0 {
			lowQuotaAlerts++
		}
	}

	resp := DashboardResponse{
		Version:         "1.0.0",
		Uptime:          formatDuration(uptime),
		UptimeSeconds:   int64(uptime.Seconds()),
		OpenNDSRunning:  openNDSRunning,
		DnsmasqRunning:  dnsmasqRunning,
		ActiveSessions:  len(sessions),
		TotalChildren:   len(children),
		MemoryUsedMB:    memUsed,
		MemoryTotalMB:   memTotal,
		MemoryPercent:   memPercent,
		CPULoad:         cpuLoad,
		DiskUsedPercent: diskPercent,
		OpenNDSClients:  ndsClients,
		LowQuotaAlerts:  lowQuotaAlerts,
	}

	JSON(w, http.StatusOK, resp)
}

// getSystemMemory reads memory info from /proc/meminfo
func (h *SystemHandler) getSystemMemory() (used, total float64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}

	var memTotal, memAvailable float64
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fmt.Sscanf(line, "MemTotal: %f kB", &memTotal)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			fmt.Sscanf(line, "MemAvailable: %f kB", &memAvailable)
		}
	}

	total = memTotal / 1024 // Convert to MB
	used = (memTotal - memAvailable) / 1024
	return used, total
}

// getCPULoad reads load average from /proc/loadavg
func (h *SystemHandler) getCPULoad() string {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "N/A"
	}
	parts := strings.Fields(string(data))
	if len(parts) >= 3 {
		return fmt.Sprintf("%s %s %s", parts[0], parts[1], parts[2])
	}
	return "N/A"
}

// getDiskUsage gets disk usage percentage for /opt
func (h *SystemHandler) getDiskUsage() float64 {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "df", "-h", "/opt")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return 0
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 5 {
		return 0
	}

	// Parse percentage (e.g., "45%")
	pctStr := strings.TrimSuffix(fields[4], "%")
	pct, _ := strconv.ParseFloat(pctStr, 64)
	return pct
}
