package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// NDSCtl wraps the ndsctl command-line tool
type NDSCtl struct {
	binaryPath string
}

// NewNDSCtl creates a new NDSCtl instance
func NewNDSCtl(binaryPath string) *NDSCtl {
	return &NDSCtl{
		binaryPath: binaryPath,
	}
}

// ClientInfo represents an authenticated client from ndsctl json output
type ClientInfo struct {
	ClientType string `json:"client_type"`
	IP         string `json:"ip"`
	MAC        string `json:"mac"`
	Token      string `json:"token"`
	State      string `json:"state"`
	Upload     int64  `json:"upload"`
	Download   int64  `json:"download"`
	Duration   int64  `json:"duration"`
}

// Auth authenticates a client
// sessionMinutes: session duration in minutes (0 = unlimited)
// uploadKbps/downloadKbps: bandwidth limits in Kbps (0 = unlimited)
func (n *NDSCtl) Auth(mac string, sessionMinutes, uploadKbps, downloadKbps int) error {
	// ndsctl auth mac sessiontimeout uploadrate downloadrate uploadquota downloadquota customstring
	args := []string{
		"auth",
		mac,
		fmt.Sprintf("%d", sessionMinutes),
		fmt.Sprintf("%d", uploadKbps),
		fmt.Sprintf("%d", downloadKbps),
		"0", // uploadquota (unlimited)
		"0", // downloadquota (unlimited)
		"",  // customstring
	}
	return n.exec(args...)
}

// Deauth removes authentication for a client
func (n *NDSCtl) Deauth(macOrIP string) error {
	return n.exec("deauth", macOrIP)
}

// Status returns the current openNDS status as a string
func (n *NDSCtl) Status() (string, error) {
	return n.execOutput("status")
}

// JSON returns all clients in JSON format
func (n *NDSCtl) JSON() ([]ClientInfo, error) {
	output, err := n.execOutput("json")
	if err != nil {
		return nil, err
	}

	var clients []ClientInfo
	if err := json.Unmarshal([]byte(output), &clients); err != nil {
		// Try to parse as object with clients array
		var wrapper struct {
			Clients []ClientInfo `json:"clients"`
		}
		if err2 := json.Unmarshal([]byte(output), &wrapper); err2 != nil {
			return nil, err
		}
		return wrapper.Clients, nil
	}
	return clients, nil
}

// IsRunning checks if openNDS is running
func (n *NDSCtl) IsRunning() bool {
	_, err := n.execOutput("status")
	return err == nil
}

// exec runs ndsctl with the given arguments
func (n *NDSCtl) exec(args ...string) error {
	cmd := exec.Command(n.binaryPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("ndsctl %s: %s", args[0], errMsg)
		}
		return fmt.Errorf("ndsctl %s: %w", args[0], err)
	}
	return nil
}

// execOutput runs ndsctl and returns stdout
func (n *NDSCtl) execOutput(args ...string) (string, error) {
	cmd := exec.Command(n.binaryPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("ndsctl %s: %s", args[0], errMsg)
		}
		return "", fmt.Errorf("ndsctl %s: %w", args[0], err)
	}
	return strings.TrimSpace(stdout.String()), nil
}
