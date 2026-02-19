package handlers

import (
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"parenta/internal/api/middleware"
	"parenta/internal/config"
	"parenta/internal/models"
	"parenta/internal/services"
	"parenta/internal/storage"
)

// FASHandler handles OpenNDS FAS authentication endpoints
type FASHandler struct {
	storage *storage.Storage
	ndsctl  *services.NDSCtl
	authSvc *services.AuthService
	config  *config.Config
	auth    *middleware.AuthMiddleware
}

// NewFASHandler creates a new FASHandler
func NewFASHandler(
	store *storage.Storage,
	ndsctl *services.NDSCtl,
	authSvc *services.AuthService,
	cfg *config.Config,
	auth *middleware.AuthMiddleware,
) *FASHandler {
	return &FASHandler{
		storage: store,
		ndsctl:  ndsctl,
		authSvc: authSvc,
		config:  cfg,
		auth:    auth,
	}
}

// FASData holds decoded FAS parameters
type FASData struct {
	HID         string
	ClientIP    string
	ClientMAC   string
	GatewayName string
	GatewayHash string
	AuthDir     string
	OriginURL   string
}

// getMACFromIP pulls the MAC address from the router's ARP table
func getMACFromIP(ip string) string {
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		// /proc/net/arp format: IP address, HW type, Flags, HW address, Mask, Device
		if len(fields) >= 4 && fields[0] == ip {
			return fields[3]
		}
	}
	return ""
}

// HandleFAS handles the initial FAS redirect from openNDS
func (h *FASHandler) HandleFAS(w http.ResponseWriter, r *http.Request) {
	fasParam := r.URL.Query().Get("fas")

	if fasParam == "" {
		log.Printf("FAS: No 'fas' param found. Redirecting to portal.")
		http.Redirect(w, r, "/portal", http.StatusFound)
		return
	}

	log.Printf("FAS: Raw fas param (first 200 chars): %.200s", fasParam)

	// 1. Normalize base64: URL query params turn '+' into spaces
	fasParam = strings.ReplaceAll(fasParam, " ", "+")

	// 2. Strip any padding issues and try multiple decode strategies
	var decodedBytes []byte
	var err error

	// Try standard base64 first
	decodedBytes, err = base64.StdEncoding.DecodeString(fasParam)
	if err != nil {
		// Try with padding fixed
		padded := fasParam
		if m := len(padded) % 4; m != 0 {
			padded += strings.Repeat("=", 4-m)
		}
		decodedBytes, err = base64.StdEncoding.DecodeString(padded)
		if err != nil {
			// Try URL-safe encoding
			decodedBytes, err = base64.URLEncoding.DecodeString(fasParam)
			if err != nil {
				// Try RawStdEncoding (no padding)
				decodedBytes, err = base64.RawStdEncoding.DecodeString(fasParam)
				if err != nil {
					log.Printf("FAS: All base64 decode attempts failed: %v", err)
					http.Redirect(w, r, "/portal", http.StatusFound)
					return
				}
			}
		}
	}

	// 3. Convert to string and clean thoroughly
	rawString := string(decodedBytes)

	// Strip null bytes and all non-printable characters
	rawString = strings.Map(func(r rune) rune {
		if r == 0 || r < 32 || r > 126 {
			return -1
		}
		return r
	}, rawString)

	// Strip OpenNDS "(null)" artifacts that appear at the end of FAS data.
	// OpenNDS appends literal "(null)" text for unset custom parameters.
	rawString = strings.TrimRight(rawString, " ,")
	for strings.Contains(rawString, "(null)") {
		rawString = strings.ReplaceAll(rawString, "(null)", "")
	}
	rawString = strings.TrimRight(rawString, " ,")

	log.Printf("FAS Decoded Data (Cleaned): %s", rawString)

	// 4. Parse the cleaned string
	fasData := h.parseFASData(rawString)

	// 5. Fallback for MAC address via ARP if parsing failed or was missing
	if fasData.ClientMAC == "" {
		clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		if fasData.ClientIP == "" {
			fasData.ClientIP = clientIP
		}
		fasData.ClientMAC = getMACFromIP(fasData.ClientIP)
		log.Printf("FAS: Auto-discovered MAC %s for IP %s via ARP", fasData.ClientMAC, fasData.ClientIP)
	}

	// 6. Normalize MAC
	fasData.ClientMAC = normalizeMAC(fasData.ClientMAC)

	// 7. URL-decode values that OpenNDS may have percent-encoded inside the base64 payload
	fasData.GatewayName = safeURLUnescape(fasData.GatewayName)
	fasData.OriginURL = safeURLUnescape(fasData.OriginURL)
	fasData.AuthDir = safeURLUnescape(fasData.AuthDir)

	// 8. Sanitize all values to ensure no control characters leak into HTTP headers
	fasData.HID = sanitizeHeaderValue(fasData.HID)
	fasData.ClientMAC = sanitizeHeaderValue(fasData.ClientMAC)
	fasData.ClientIP = sanitizeHeaderValue(fasData.ClientIP)
	fasData.GatewayName = sanitizeHeaderValue(fasData.GatewayName)
	fasData.AuthDir = sanitizeHeaderValue(fasData.AuthDir)
	fasData.OriginURL = sanitizeHeaderValue(fasData.OriginURL)

	log.Printf("FAS Parsed: hid=%s mac=%s ip=%s gw=%s originurl=%s",
		fasData.HID, fasData.ClientMAC, fasData.ClientIP, fasData.GatewayName, fasData.OriginURL)

	// 9. Safely construct the redirect URL using url.Values
	redirectParams := url.Values{}
	redirectParams.Set("hid", fasData.HID)
	redirectParams.Set("mac", fasData.ClientMAC)
	redirectParams.Set("ip", fasData.ClientIP)
	redirectParams.Set("gatewayname", fasData.GatewayName)
	redirectParams.Set("authdir", fasData.AuthDir)
	redirectParams.Set("originurl", fasData.OriginURL)

	portalURL := fmt.Sprintf("http://%s:%v/portal?%s",
		h.config.OpenNDS.GatewayIP,
		h.config.Server.Port,
		redirectParams.Encode(),
	)

	log.Printf("FAS Redirect URL: %s", portalURL)

	// 10. Execute redirect
	http.Redirect(w, r, portalURL, http.StatusFound)
}

// safeURLUnescape decodes percent-encoded strings, returning original on error
func safeURLUnescape(s string) string {
	if unescaped, err := url.QueryUnescape(s); err == nil {
		return unescaped
	}
	return s
}

// sanitizeHeaderValue removes any characters that could corrupt HTTP headers.
// Only allows printable ASCII (space through tilde), no control chars or null bytes.
func sanitizeHeaderValue(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 32 && r <= 126 {
			return r
		}
		return -1
	}, s)
}

// parseFASData parses the FAS query string (Level 1 format: "key1=var1, key2=var2, ...")
func (h *FASHandler) parseFASData(data string) FASData {
	var fas FASData
	
	pairs := strings.Split(data, ", ")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		
		key := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])

		// Skip empty values and OpenNDS null markers
		if value == "" || value == "(null)" {
			continue
		}

		switch key {
		case "hid":
			fas.HID = value
		case "clientip":
			fas.ClientIP = value
		case "clientmac":
			fas.ClientMAC = value
		case "gatewayname":
			fas.GatewayName = value
		case "gatewayhash":
			fas.GatewayHash = value
		case "authdir":
			fas.AuthDir = value
		case "originurl":
			fas.OriginURL = value
		}
	}
	return fas
}

// AuthRequest represents child login form submission
type AuthRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	HID       string `json:"hid"`
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	AuthDir   string `json:"authdir"`
	OriginURL string `json:"originurl"`
}

// HandleAuth processes login from captive portal (supports both admin and child)
func (h *FASHandler) HandleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		if r.Method == http.MethodGet {
			http.Redirect(w, r, "/portal", http.StatusFound)
			return
		}
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse form or JSON
	var req AuthRequest
	contentType := r.Header.Get("Content-Type")
	isJSON := strings.Contains(contentType, "application/json")

	if isJSON {
		if err := ParseJSON(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request")
			return
		}
	} else {
		// Form submission
		r.ParseForm()
		req = AuthRequest{
			Username:  r.FormValue("username"),
			Password:  r.FormValue("password"),
			HID:       r.FormValue("hid"),
			MAC:       r.FormValue("mac"),
			IP:       r.FormValue("ip"),
			AuthDir:   r.FormValue("authdir"),
			OriginURL: r.FormValue("originurl"),
		}
	}

	// Auto-discover MAC via ARP if missing (Plug & Play Rescue)
	if req.MAC == "" {
		clientIP := req.IP
		if clientIP == "" {
			clientIP, _, _ = net.SplitHostPort(r.RemoteAddr)
		}
		req.MAC = getMACFromIP(clientIP)
		if req.MAC != "" {
			log.Printf("Auth: Auto-discovered MAC %s for IP %s via ARP", req.MAC, clientIP)
		}
	}

	// Validate MAC format if provided
	if req.MAC != "" {
		req.MAC = normalizeMAC(req.MAC)
	}

	// Try admin authentication first
	admin, err := h.authSvc.AuthenticateAdmin(req.Username, req.Password)
	if err == nil {
		// Admin success - generate JWT
		token, err := h.auth.GenerateToken(admin.ID, admin.Username, true, h.config.Session.JWTExpiryHours)
		if err != nil {
			log.Printf("Failed to generate token for admin %s: %v", admin.Username, err)
			Error(w, http.StatusInternalServerError, "authentication error")
			return
		}

		// Grant internet access via OpenNDS if MAC provided
		if req.MAC != "" {
			if err := h.ndsctl.Deauth(req.MAC); err != nil {
				log.Printf("Pre-deauth failed for MAC %s: %v", req.MAC, err)
			}
			time.Sleep(100 * time.Millisecond)

			if err := h.ndsctl.Auth(req.MAC, 0, 0, 0); err != nil {
				log.Printf("ndsctl auth failed for admin %s (MAC: %s): %v", admin.Username, req.MAC, err)
			} else {
				log.Printf("Admin %s authenticated on MAC %s with unlimited access", admin.Username, req.MAC)
			}
		} else {
			log.Printf("Admin %s logged in without MAC (dashboard only)", admin.Username)
		}

		if isJSON {
			JSON(w, http.StatusOK, map[string]interface{}{
				"type":                  "admin",
				"token":                 token,
				"expires_in":            h.config.Session.JWTExpiryHours * 3600,
				"force_password_change": admin.ForcePasswordChange,
			})
		} else {
			redirectURL := fmt.Sprintf("/portal?auth_type=admin&token=%s&force_password_change=%t",
				url.QueryEscape(token), admin.ForcePasswordChange)
			http.Redirect(w, r, redirectURL, http.StatusFound)
		}
		return
	}

	// Try child authentication
	child, err := h.authSvc.AuthenticateChild(req.Username, req.Password)
	if err != nil {
		log.Printf("Failed login attempt for username: %s from IP: %s MAC: %s", req.Username, req.IP, req.MAC)
		if isJSON {
			Error(w, http.StatusUnauthorized, "Invalid username or password")
		} else {
			errorURL := fmt.Sprintf("/portal?hid=%s&mac=%s&ip=%s&authdir=%s&originurl=%s&error=%s",
				url.QueryEscape(req.HID), url.QueryEscape(req.MAC), url.QueryEscape(req.IP),
				url.QueryEscape(req.AuthDir), url.QueryEscape(req.OriginURL), url.QueryEscape("Invalid username or password"))
			http.Redirect(w, r, errorURL, http.StatusFound)
		}
		return
	}

	log.Printf("Child %s (ID: %s) attempting login from MAC: %s IP: %s", child.Name, child.ID, req.MAC, req.IP)

	if child.RemainingMinutes() <= 0 {
		log.Printf("Child %s denied: no time remaining", child.Name)
		if isJSON {
			Error(w, http.StatusForbidden, "No time remaining for today")
		} else {
			errorURL := fmt.Sprintf("/portal?hid=%s&mac=%s&ip=%s&authdir=%s&originurl=%s&error=%s",
				url.QueryEscape(req.HID), url.QueryEscape(req.MAC), url.QueryEscape(req.IP),
				url.QueryEscape(req.AuthDir), url.QueryEscape(req.OriginURL), url.QueryEscape("No time remaining for today"))
			http.Redirect(w, r, errorURL, http.StatusFound)
		}
		return
	}

	if child.ScheduleID != "" {
		schedule := h.storage.GetSchedule(child.ScheduleID)
		if schedule != nil && !schedule.IsAllowedNow() {
			if isJSON {
				Error(w, http.StatusForbidden, "Internet access not allowed at this time")
			} else {
				errorURL := fmt.Sprintf("/portal?hid=%s&mac=%s&ip=%s&authdir=%s&originurl=%s&error=%s",
					url.QueryEscape(req.HID), url.QueryEscape(req.MAC), url.QueryEscape(req.IP),
					url.QueryEscape(req.AuthDir), url.QueryEscape(req.OriginURL), url.QueryEscape("Internet access not allowed at this time"))
				http.Redirect(w, r, errorURL, http.StatusFound)
			}
			return
		}
	}

	if req.MAC != "" && !child.HasDevice(req.MAC) {
		deviceName := fmt.Sprintf("Device %d", len(child.Devices)+1)
		child.AddDevice(req.MAC, deviceName)
		h.storage.SaveChild(child)
	}

	session := &models.Session{
		ID:        services.GenerateID(),
		ChildID:   child.ID,
		ChildName: child.Name,
		MAC:       req.MAC,
		IP:        req.IP,
		StartedAt: time.Now(),
		IsActive:  true,
	}
	h.storage.SaveSession(session)

	remainingMin := child.RemainingMinutes()

	if req.MAC != "" {
		_ = h.ndsctl.Deauth(req.MAC)
		time.Sleep(50 * time.Millisecond)

		if err := h.ndsctl.Auth(req.MAC, remainingMin, 0, 0); err != nil {
			log.Printf("ndsctl auth failed for child %s (MAC: %s): %v", child.Name, req.MAC, err)
		} else {
			log.Printf("Child %s authenticated on MAC %s with %d minutes", child.Name, req.MAC, remainingMin)
		}
	}

	if isJSON {
		JSON(w, http.StatusOK, map[string]interface{}{
			"type":              "child",
			"child_name":        child.Name,
			"remaining_minutes": remainingMin,
			"redirect_url":      req.OriginURL,
		})
	} else {
		redirectTarget := req.OriginURL
		if redirectTarget == "" || redirectTarget == "null" {
			redirectTarget = "http://" + h.config.OpenNDS.GatewayIP + ":8080/portal?success=1"
		}
		redirectURL := fmt.Sprintf("%s&child_name=%s&remaining=%d",
			redirectTarget, url.QueryEscape(child.Name), remainingMin)
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

// normalizeMAC standardizes MAC address format
func normalizeMAC(mac string) string {
	mac = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(mac, ":", ""), "-", ""), ".", ""))
	if len(mac) == 12 {
		var parts []string
		for i := 0; i < 12; i += 2 {
			parts = append(parts, mac[i:i+2])
		}
		return strings.Join(parts, ":")
	}
	return mac
}

// HandleStatus shows remaining time for a logged-in client
func (h *FASHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	mac := r.URL.Query().Get("mac")
	if mac == "" {
		Error(w, http.StatusBadRequest, "missing mac parameter")
		return
	}

	session := h.storage.GetSessionByMAC(mac)
	if session == nil || !session.IsActive {
		Error(w, http.StatusNotFound, "no active session")
		return
	}

	child := h.storage.GetChild(session.ChildID)
	if child == nil {
		Error(w, http.StatusNotFound, "child not found")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"child_name":        child.Name,
		"remaining_minutes": child.RemainingMinutes(),
		"used_today":        child.UsedTodayMin,
		"daily_quota":       child.DailyQuotaMin,
		"session_start":     session.StartedAt,
	})
}