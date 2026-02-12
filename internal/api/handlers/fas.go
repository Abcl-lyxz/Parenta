package handlers

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

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
}

// NewFASHandler creates a new FASHandler
func NewFASHandler(
	store *storage.Storage,
	ndsctl *services.NDSCtl,
	authSvc *services.AuthService,
	cfg *config.Config,
) *FASHandler {
	return &FASHandler{
		storage: store,
		ndsctl:  ndsctl,
		authSvc: authSvc,
		config:  cfg,
	}
}

// FASData holds decoded FAS parameters
type FASData struct {
	HID         string // Hash ID from openNDS
	ClientIP    string
	ClientMAC   string
	GatewayName string
	GatewayHash string
	AuthDir     string
	OriginURL   string
}

// HandleFAS handles the initial FAS redirect from openNDS
func (h *FASHandler) HandleFAS(w http.ResponseWriter, r *http.Request) {
	// Get the fas query parameter (base64 encoded)
	fasParam := r.URL.Query().Get("fas")
	if fasParam == "" {
		Error(w, http.StatusBadRequest, "missing fas parameter")
		return
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(fasParam)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid fas parameter")
		return
	}

	// Parse the decoded data (format: key=value&key2=value2)
	fasData := h.parseFASData(string(decoded))

	// Redirect to login portal with parameters
	portalURL := fmt.Sprintf("/portal?hid=%s&mac=%s&ip=%s&gatewayname=%s&authdir=%s&originurl=%s",
		url.QueryEscape(fasData.HID),
		url.QueryEscape(fasData.ClientMAC),
		url.QueryEscape(fasData.ClientIP),
		url.QueryEscape(fasData.GatewayName),
		url.QueryEscape(fasData.AuthDir),
		url.QueryEscape(fasData.OriginURL),
	)

	http.Redirect(w, r, portalURL, http.StatusFound)
}

// parseFASData parses the FAS query string
func (h *FASHandler) parseFASData(data string) FASData {
	var fas FASData
	pairs := strings.Split(data, ", ")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

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

// HandlePortal serves the captive portal login page
func (h *FASHandler) HandlePortal(w http.ResponseWriter, r *http.Request) {
	// Get all children for the login form
	children := h.storage.ListChildren()

	data := struct {
		HID         string
		MAC         string
		IP          string
		GatewayName string
		AuthDir     string
		OriginURL   string
		Children    []*models.Child
		Error       string
	}{
		HID:         r.URL.Query().Get("hid"),
		MAC:         r.URL.Query().Get("mac"),
		IP:          r.URL.Query().Get("ip"),
		GatewayName: r.URL.Query().Get("gatewayname"),
		AuthDir:     r.URL.Query().Get("authdir"),
		OriginURL:   r.URL.Query().Get("originurl"),
		Children:    children,
		Error:       r.URL.Query().Get("error"),
	}

	// Serve the portal page
	tmpl := template.Must(template.New("portal").Parse(portalTemplate))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, data)
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

// HandleAuth processes child login from captive portal
func (h *FASHandler) HandleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse form or JSON
	var req AuthRequest
	contentType := r.Header.Get("Content-Type")

	if strings.Contains(contentType, "application/json") {
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
			IP:        r.FormValue("ip"),
			AuthDir:   r.FormValue("authdir"),
			OriginURL: r.FormValue("originurl"),
		}
	}

	// Authenticate child
	child, err := h.authSvc.AuthenticateChild(req.Username, req.Password)
	if err != nil {
		// Redirect back to portal with error
		errorURL := fmt.Sprintf("/portal?hid=%s&mac=%s&ip=%s&authdir=%s&originurl=%s&error=%s",
			url.QueryEscape(req.HID),
			url.QueryEscape(req.MAC),
			url.QueryEscape(req.IP),
			url.QueryEscape(req.AuthDir),
			url.QueryEscape(req.OriginURL),
			url.QueryEscape("Invalid username or password"),
		)
		http.Redirect(w, r, errorURL, http.StatusFound)
		return
	}

	// Check quota
	if child.RemainingMinutes() <= 0 {
		errorURL := fmt.Sprintf("/portal?hid=%s&mac=%s&ip=%s&authdir=%s&originurl=%s&error=%s",
			url.QueryEscape(req.HID),
			url.QueryEscape(req.MAC),
			url.QueryEscape(req.IP),
			url.QueryEscape(req.AuthDir),
			url.QueryEscape(req.OriginURL),
			url.QueryEscape("No time remaining for today"),
		)
		http.Redirect(w, r, errorURL, http.StatusFound)
		return
	}

	// Check schedule
	if child.ScheduleID != "" {
		schedule := h.storage.GetSchedule(child.ScheduleID)
		if schedule != nil && !schedule.IsAllowedNow() {
			errorURL := fmt.Sprintf("/portal?hid=%s&mac=%s&ip=%s&authdir=%s&originurl=%s&error=%s",
				url.QueryEscape(req.HID),
				url.QueryEscape(req.MAC),
				url.QueryEscape(req.IP),
				url.QueryEscape(req.AuthDir),
				url.QueryEscape(req.OriginURL),
				url.QueryEscape("Internet access not allowed at this time"),
			)
			http.Redirect(w, r, errorURL, http.StatusFound)
			return
		}
	}

	// Auto-discover device if new
	if !child.HasDevice(req.MAC) {
		child.AddDevice(req.MAC, "Auto-discovered device")
		h.storage.SaveChild(child)
	}

	// Create session
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

	// Authenticate with openNDS
	remainingMin := child.RemainingMinutes()
	if err := h.ndsctl.Auth(req.MAC, remainingMin, 0, 0); err != nil {
		// Log error but continue - client might still get through
		fmt.Printf("ndsctl auth error: %v\n", err)
	}

	// Redirect to original URL or success page
	if req.OriginURL != "" && req.OriginURL != "null" {
		http.Redirect(w, r, req.OriginURL, http.StatusFound)
	} else {
		// Show success page
		h.showSuccessPage(w, child, remainingMin)
	}
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

// showSuccessPage displays a success page after login
func (h *FASHandler) showSuccessPage(w http.ResponseWriter, child *models.Child, remainingMin int) {
	tmpl := template.Must(template.New("success").Parse(successTemplate))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, map[string]interface{}{
		"Name":      child.Name,
		"Remaining": remainingMin,
	})
}

// Portal HTML template
const portalTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Parenta - Login</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #fff;
            color: #1a1a1a;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 1rem;
        }
        .container {
            width: 100%;
            max-width: 400px;
            border: 1px solid #e0e0e0;
            padding: 2rem;
        }
        h1 {
            font-size: 1.5rem;
            font-weight: 700;
            letter-spacing: 0.1em;
            margin-bottom: 1.5rem;
            text-align: center;
        }
        .error {
            background: #f5f5f5;
            border: 1px solid #1a1a1a;
            padding: 0.75rem;
            margin-bottom: 1rem;
            font-size: 0.9rem;
        }
        label {
            display: block;
            margin-bottom: 0.5rem;
            font-weight: 500;
        }
        input {
            width: 100%;
            padding: 0.75rem;
            border: 1px solid #e0e0e0;
            margin-bottom: 1rem;
            font-size: 1rem;
        }
        input:focus {
            outline: none;
            border-color: #1a1a1a;
        }
        button {
            width: 100%;
            padding: 0.75rem;
            background: #1a1a1a;
            color: #fff;
            border: none;
            cursor: pointer;
            font-size: 1rem;
            font-weight: 500;
        }
        button:hover {
            opacity: 0.9;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>PARENTA</h1>
        {{if .Error}}
        <div class="error">{{.Error}}</div>
        {{end}}
        <form method="POST" action="/fas/auth">
            <input type="hidden" name="hid" value="{{.HID}}">
            <input type="hidden" name="mac" value="{{.MAC}}">
            <input type="hidden" name="ip" value="{{.IP}}">
            <input type="hidden" name="authdir" value="{{.AuthDir}}">
            <input type="hidden" name="originurl" value="{{.OriginURL}}">

            <label for="username">Username</label>
            <input type="text" id="username" name="username" required autofocus>

            <label for="password">Password</label>
            <input type="password" id="password" name="password" required>

            <button type="submit">Connect to Internet</button>
        </form>
    </div>
</body>
</html>`

// Success HTML template
const successTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Connected</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #fff;
            color: #1a1a1a;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 1rem;
        }
        .container {
            text-align: center;
            max-width: 400px;
        }
        h1 {
            font-size: 1.5rem;
            margin-bottom: 1rem;
        }
        p {
            font-size: 1.1rem;
            margin-bottom: 0.5rem;
        }
        .remaining {
            font-size: 2rem;
            font-weight: 700;
            margin: 1rem 0;
        }
        .note {
            color: #666;
            font-size: 0.9rem;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Welcome, {{.Name}}!</h1>
        <p>You are now connected to the internet.</p>
        <div class="remaining">{{.Remaining}} minutes remaining</div>
        <p class="note">You can close this window and start browsing.</p>
    </div>
</body>
</html>`
