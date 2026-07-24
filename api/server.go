package api

import (
	"log"
	"net/http"
	"strconv"
	"time"
)

func StartAPIServer(port int, token string) {
	GlobalToken = token

	mux := http.NewServeMux()

	// API Routes
	mux.HandleFunc("/api/users", handleApiUsers)
	mux.HandleFunc("/api/usage", handleApiUsage)
	mux.HandleFunc("/api/online", handleApiOnline)
	mux.HandleFunc("/api/internal/online", handleApiInternalOnline)

	// Admin Data Routes
	mux.HandleFunc("/admin/api/live-data", handleLiveData)

	// Admin Page Routes
	mux.HandleFunc("/admin/login", handleLogin)
	mux.HandleFunc("/admin/logout", handleLogout)
	mux.HandleFunc("/admin/dashboard", handleDashboard)
	mmux_registerChartFull(mux)
	mux.HandleFunc("/admin/drilldown", handleDrilldownPage)
	mux.HandleFunc("/admin/node-chart", handleNodeChart)

	// Admin Actions
	mux.HandleFunc("/admin/actions", handleActions)
	mux.HandleFunc("/admin/backup/download", handleBackupDownload)
	mux.HandleFunc("/admin/backup/restore", handleBackupRestore)

	// Client Routes
	mux.HandleFunc("/sub/", handleSub)

	// Static assets (logo, etc.)
	mux.HandleFunc("/static/", handleStatic)

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("\U0001F680 Control Panel API Server listening on port %d...\n", port)
	log.Fatal(srv.ListenAndServe())
}

// mmux_registerChartFull keeps the chart-full route registration in one place
// without touching the rest of the file.
func mmux_registerChartFull(mux *http.ServeMux) {
	mux.HandleFunc("/admin/chart-full", handleChartFull)
}
