package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Global Variables
var GlobalToken string

// Data Models
type UsagePayload struct {
	Username   string `json:"username"`
	BytesAdded int64  `json:"bytes_added"`
}

type NPVConfig struct {
	SSHConfigType       string `json:"sshConfigType"`
	Remarks             string `json:"remarks"`
	SSHHost             string `json:"sshHost"`
	SSHPort             int    `json:"sshPort"`
	SSHUsername         string `json:"sshUsername"`
	SSHPassword         string `json:"sshPassword"`
	UDPGWPort           int    `json:"udpgwPort"`
	UDPGWTransparentDNS bool   `json:"udpgwTransparentDNS"`
}

type WebUser struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	DataUsed   int64  `json:"data_used"`
	DataLimit  int64  `json:"data_limit"`
	ExpiryUnix int64  `json:"expiry_unix"`
	LastSeen   int64  `json:"last_seen"`
	SubToken   string `json:"sub_token"`
	UdpgwPort  int    `json:"udpgw"`
	Comment    string `json:"comment"`
}

type WebNode struct {
	IP           string `json:"IP"`
	LastSeen     int64  `json:"LastSeen"`
	TotalTraffic int64  `json:"TotalTraffic"`
	Domain       string `json:"Domain"`
	CustomRemark string `json:"CustomRemark"`
	IsOnline     bool   `json:"IsOnline"`
}

// ---------------------------------------------------------------------------
// Admin session store (random server-side tokens, replaces the old hard-coded
// "authenticated_admin_session" cookie value that let anyone forge a session).
// ---------------------------------------------------------------------------

const sessionCookieName = "svm_session"
const sessionTTL = 24 * time.Hour

var (
	sessions   = make(map[string]time.Time) // token -> expiry
	sessionsMu sync.Mutex
)

func newSessionToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	sessionsMu.Lock()
	sessions[token] = time.Now().Add(sessionTTL)
	sessionsMu.Unlock()
	return token
}

func validSession(token string) bool {
	if token == "" {
		return false
	}
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	exp, ok := sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(sessions, token)
		return false
	}
	return true
}

func destroySession(token string) {
	sessionsMu.Lock()
	delete(sessions, token)
	sessionsMu.Unlock()
}

// isSecureRequest reports whether the original client request used HTTPS
// (directly or via a reverse proxy).
func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(sessionTTL),
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
	})
}

// Auth & Security
func checkAdminAuth(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	return validSession(cookie.Value)
}

// checkNodeToken validates the cluster token from either the Authorization
// header ("Bearer <token>" / raw), the X-Auth-Token header, or the legacy
// ?token= query parameter, using a constant-time comparison.
func checkNodeToken(r *http.Request) bool {
	provided := r.Header.Get("X-Auth-Token")
	if provided == "" {
		if auth := r.Header.Get("Authorization"); auth != "" {
			provided = strings.TrimPrefix(auth, "Bearer ")
		}
	}
	if provided == "" {
		provided = r.URL.Query().Get("token")
	}
	if provided == "" || GlobalToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(GlobalToken)) == 1
}

// Helper Functions
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func getSystemRAM() float64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	var total, free, cached, buffers int64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseInt(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = val
		case "MemFree:":
			free = val
		case "Cached:":
			cached = val
		case "Buffers:":
			buffers = val
		}
	}
	if total == 0 {
		return 0
	}
	used := total - (free + cached + buffers)
	return (float64(used) / float64(total)) * 100
}

func getSystemCPU() float64 {
	getStat := func() (int64, int64) {
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0
		}
		lines := strings.Split(string(data), "\n")
		if len(lines) == 0 {
			return 0, 0
		}
		fields := strings.Fields(lines[0])
		if len(fields) < 5 {
			return 0, 0
		}
		var total int64
		for i := 1; i < len(fields); i++ {
			v, _ := strconv.ParseInt(fields[i], 10, 64)
			total += v
		}
		idle, _ := strconv.ParseInt(fields[4], 10, 64)
		iowait, _ := strconv.ParseInt(fields[5], 10, 64)
		return total, idle + iowait
	}
	t1, i1 := getStat()
	time.Sleep(200 * time.Millisecond)
	t2, i2 := getStat()
	deltaTotal := t2 - t1
	deltaIdle := i2 - i1
	if deltaTotal <= 0 {
		return 0
	}
	return float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100
}

// ---------------------------------------------------------------------------
// GeoIP lookup with in-memory caching (avoids hitting ip-api.com on every
// page load for every node, which was slow and rate-limited).
// ---------------------------------------------------------------------------

type geoEntry struct {
	country string
	flag    string
	at      time.Time
}

var (
	geoCache   = make(map[string]geoEntry)
	geoCacheMu sync.Mutex
)

const geoCacheTTL = 12 * time.Hour

func getCountryFlag(ip string) (string, string) {
	if ip == "Main-Server" || ip == "127.0.0.1" || strings.HasPrefix(ip, "192.168") || strings.HasPrefix(ip, "10.") {
		return "Local", "\U0001F30D"
	}

	geoCacheMu.Lock()
	if e, ok := geoCache[ip]; ok && time.Since(e.at) < geoCacheTTL {
		geoCacheMu.Unlock()
		return e.country, e.flag
	}
	geoCacheMu.Unlock()

	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://ip-api.com/json/" + ip)
	if err != nil {
		return "Unknown", "\U0001F30D"
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	_ = json.Unmarshal(body, &result)
	countryCode, ok := result["countryCode"].(string)
	if !ok {
		return "Unknown", "\U0001F30D"
	}
	countryName, _ := result["country"].(string)
	flag := ""
	if len(countryCode) == 2 {
		flag = string([]rune{rune(countryCode[0]) + 127397, rune(countryCode[1]) + 127397})
	}

	geoCacheMu.Lock()
	geoCache[ip] = geoEntry{country: countryName, flag: flag, at: time.Now()}
	geoCacheMu.Unlock()

	return countryName, flag
}

// Mobile Detector
func isMobile(r *http.Request) bool {
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	return strings.Contains(ua, "mobile") || strings.Contains(ua, "android") || strings.Contains(ua, "iphone")
}

// parseLogDate tolerates a log_date value that arrives either as a plain date
// ("2006-01-02") or as an RFC3339 timestamp (which is what database/sql yields
// when a DATE column is scanned into a string while parseTime=true).
func parseLogDate(s string, loc *time.Location) (time.Time, error) {
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized date %q", s)
}
