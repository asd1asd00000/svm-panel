package api

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/asd1asd00000/svm-panel/database"
	"github.com/asd1asd00000/svm-panel/models"
	"github.com/asd1asd00000/svm-panel/sshvpn"
)

func handleLiveData(w http.ResponseWriter, r *http.Request) {
	if !checkAdminAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	users, _ := database.GetUsers()
	nodes, _ := database.GetNodes()

	onlineCount := 0
	liveOnlineMap := make(map[string]bool)
	for _, u := range sshvpn.GetOnlineUsersList() {
		liveOnlineMap[u] = true
		onlineCount++
	}

	inactiveCount := 0
	nowUnix := time.Now().Unix()
	var webUsers []WebUser
	for _, u := range users {
		if !u.IsActive() {
			inactiveCount++
		}
		webUsers = append(webUsers, WebUser{
			Username: u.Username, Password: u.Password, DataUsed: u.DataUsed,
			DataLimit: u.DataLimit, ExpiryUnix: u.ExpiryDate.Unix(),
			LastSeen: u.LastSeen, SubToken: u.SubToken, UdpgwPort: u.UdpgwPort, Comment: u.Comment,
		})
	}
	for i, j := 0, len(webUsers)-1; i < j; i, j = i+1, j-1 {
		webUsers[i], webUsers[j] = webUsers[j], webUsers[i]
	}

	activeNodesCount := 0
	var webNodes []WebNode
	for _, n := range nodes {
		if (nowUnix - n.LastSeen) < 120 {
			activeNodesCount++
		}
		webNodes = append(webNodes, WebNode{
			IP: n.IP, LastSeen: n.LastSeen, TotalTraffic: n.TotalTraffic,
			Domain: n.Domain, CustomRemark: n.CustomRemark, IsOnline: (nowUnix - n.LastSeen) < 120,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"cpu": getSystemCPU(), "ram": getSystemRAM(),
		"users": webUsers, "nodes": webNodes, "onlineMap": liveOnlineMap,
		"stats": map[string]interface{}{
			"totalUsers": len(users), "onlineUsers": onlineCount,
			"inactiveUsers": inactiveCount, "totalNodes": len(nodes), "activeNodes": activeNodesCount,
		},
	})
}

func handleDrilldownPage(w http.ResponseWriter, r *http.Request) {
	if !checkAdminAuth(r) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	drillType := r.URL.Query().Get("type")
	count, err := strconv.Atoi(r.URL.Query().Get("count"))
	if err != nil || count <= 0 {
		http.Error(w, "Invalid parameters", http.StatusBadRequest)
		return
	}

	nodes, _ := database.GetNodes()
	nodeName := func(ip string) string {
		for _, n := range nodes {
			if n.IP == ip && n.CustomRemark != "" {
				return n.CustomRemark
			}
		}
		return ip
	}

	loc := models.IranTime
	now := time.Now().In(loc)
	var categories []string
	seriesMap := make(map[string][]float64)
	pageTitle := ""

	switch drillType {
	case "hourly":
		pageTitle = fmt.Sprintf("\u062a\u0641\u06a9\u06cc\u06a9 \u0633\u0627\u0639\u062a\u06cc \u2014 %d \u0633\u0627\u0639\u062a \u06af\u0630\u0634\u062a\u0647", count)
		startTime := now.Add(-time.Duration(count) * time.Hour).Truncate(time.Hour)
		rows, err := database.DB.Query(`
			SELECT node_ip, DATE_FORMAT(log_date, '%Y-%m-%d') AS log_date, log_hour, SUM(bytes_used) AS total
			FROM traffic_logs
			WHERE TIMESTAMP(log_date, MAKETIME(log_hour, 0, 0)) >= ?
			GROUP BY node_ip, log_date, log_hour`, startTime.Format("2006-01-02 15:00:00"))

		if err == nil {
			defer rows.Close()
			for i := count - 1; i >= 0; i-- {
				categories = append(categories, now.Add(-time.Duration(i)*time.Hour).Format("15:00"))
			}
			truncatedNow := now.Truncate(time.Hour)
			for rows.Next() {
				var ip, logDate string
				var logHour int
				var total int64
				if rows.Scan(&ip, &logDate, &logHour, &total) != nil {
					continue
				}
				t, err := parseLogDate(logDate, loc)
				if err != nil {
					continue
				}
				t = t.Add(time.Duration(logHour) * time.Hour)
				hourDiff := int(truncatedNow.Sub(t).Hours())
				idx := count - 1 - hourDiff
				name := nodeName(ip)
				if _, ok := seriesMap[name]; !ok {
					seriesMap[name] = make([]float64, count)
				}
				if idx >= 0 && idx < count {
					seriesMap[name][idx] += float64(total) / float64(models.GB)
				}
			}
		}
	case "daily":
		pageTitle = fmt.Sprintf("\u062a\u0641\u06a9\u06cc\u06a9 \u0631\u0648\u0632\u0627\u0646\u0647 \u2014 %d \u0631\u0648\u0632 \u06af\u0630\u0634\u062a\u0647", count)
		startDate := now.AddDate(0, 0, -count+1).Format("2006-01-02")
		rows, err := database.DB.Query(`
			SELECT node_ip, DATE_FORMAT(log_date, '%Y-%m-%d') AS log_date, SUM(bytes_used) AS total
			FROM traffic_logs
			WHERE log_date >= ?
			GROUP BY node_ip, log_date`, startDate)

		if err == nil {
			defer rows.Close()
			for i := count - 1; i >= 0; i-- {
				categories = append(categories, now.AddDate(0, 0, -i).Format("01/02"))
			}
			y, m, d := now.Date()
			startOfToday := time.Date(y, m, d, 0, 0, 0, 0, loc)
			for rows.Next() {
				var ip, logDate string
				var total int64
				if rows.Scan(&ip, &logDate, &total) != nil {
					continue
				}
				pd, err := parseLogDate(logDate, loc)
				if err != nil {
					continue
				}
				dayDiff := int(startOfToday.Sub(pd).Hours() / 24)
				idx := count - 1 - dayDiff
				name := nodeName(ip)
				if _, ok := seriesMap[name]; !ok {
					seriesMap[name] = make([]float64, count)
				}
				if idx >= 0 && idx < count {
					seriesMap[name][idx] += float64(total) / float64(models.GB)
				}
			}
		}
	}

	type Series struct {
		Name string    `json:"name"`
		Data []float64 `json:"data"`
	}
	var series []Series
	for name, data := range seriesMap {
		series = append(series, Series{Name: name, Data: data})
	}
	if len(series) == 0 {
		series = append(series, Series{Name: "\u0628\u062f\u0648\u0646 \u062a\u0631\u0627\u0641\u06cc\u06a9", Data: make([]float64, count)})
	}

	categoriesJSON, _ := json.Marshal(categories)
	seriesJSON, _ := json.Marshal(series)

	fmt.Fprint(w, renderDrilldownHTML(pageTitle, string(categoriesJSON), string(seriesJSON)))
}

func handleNodeChart(w http.ResponseWriter, r *http.Request) {
	if !checkAdminAuth(r) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	nodeIP := r.URL.Query().Get("ip")
	nodes, _ := database.GetNodes()
	nodeName := nodeIP
	for _, n := range nodes {
		if n.IP == nodeIP && n.CustomRemark != "" {
			nodeName = n.CustomRemark + " (" + nodeIP + ")"
			break
		}
	}
	raw := buildNodeChartRaw(nodeIP, nodeName)
	rawBytes, _ := json.Marshal(raw)
	fmt.Fprint(w, renderNodeChartHTML(nodeName, string(rawBytes)))
}

func handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	if !checkAdminAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	user, pass := database.DBCredentials()
	cnfPath, cleanupCnf, err := database.WriteMySQLDefaultsFile(user, pass)
	if err != nil {
		database.WriteSystemLog("ERROR", "Web UI: Backup download failed - cannot prepare credentials: "+err.Error())
		http.Error(w, "Backup Error", 500)
		return
	}
	defer cleanupCnf()

	out, err := exec.Command("mysqldump", "--defaults-extra-file="+cnfPath, "svm_db").Output()
	if err != nil {
		database.WriteSystemLog("ERROR", "Web UI: Backup download failed - mysqldump error: "+err.Error())
		http.Error(w, "Backup Error", 500)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=svm_backup_"+time.Now().Format("20060102_150405")+".sql")
	w.Header().Set("Content-Type", "application/sql")
	w.Write(out)
	database.WriteSystemLog("INFO", "Web UI: Local backup downloaded.")
}

func handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	if !checkAdminAuth(r) || r.Method != http.MethodPost {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	redirect := func(suffix string) {
		http.Redirect(w, r, "/admin/dashboard?tab=settings"+suffix, http.StatusSeeOther)
	}

	file, _, err := r.FormFile("backup_file")
	if err != nil {
		database.WriteSystemLog("ERROR", "Web UI: Restore failed - no backup file uploaded.")
		redirect("&error=backup_restore_failed")
		return
	}
	defer file.Close()

	tempFile, err := os.CreateTemp("/tmp", "restore-*.sql")
	if err != nil {
		database.WriteSystemLog("ERROR", "Web UI: Restore failed - cannot create temp file: "+err.Error())
		redirect("&error=backup_restore_failed")
		return
	}
	defer os.Remove(tempFile.Name())

	if _, err := io.Copy(tempFile, file); err != nil {
		tempFile.Close()
		database.WriteSystemLog("ERROR", "Web UI: Restore failed - cannot read uploaded file: "+err.Error())
		redirect("&error=backup_restore_failed")
		return
	}
	tempFile.Close()

	in, err := os.Open(tempFile.Name())
	if err != nil {
		database.WriteSystemLog("ERROR", "Web UI: Restore failed - cannot open temp file: "+err.Error())
		redirect("&error=backup_restore_failed")
		return
	}

	user, pass := database.DBCredentials()
	cnfPath, cleanupCnf, err := database.WriteMySQLDefaultsFile(user, pass)
	if err != nil {
		in.Close()
		database.WriteSystemLog("ERROR", "Web UI: Restore failed - cannot prepare credentials: "+err.Error())
		redirect("&error=backup_restore_failed")
		return
	}
	defer cleanupCnf()

	cmd := exec.Command("mysql", "--defaults-extra-file="+cnfPath, "svm_db")
	cmd.Stdin = in
	runErr := cmd.Run()
	in.Close()
	if runErr != nil {
		database.WriteSystemLog("ERROR", "Web UI: Restore failed - mysql import error: "+runErr.Error())
		redirect("&error=backup_restore_failed")
		return
	}

	database.WriteSystemLog("INFO", "Web UI: Database restored successfully.")
	redirect("&success=backup_restored")
}

func handleApiUsers(w http.ResponseWriter, r *http.Request) {
	if !checkNodeToken(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	users, _ := database.GetUsers()
	var syncList []map[string]interface{}
	for _, u := range users {
		syncList = append(syncList, map[string]interface{}{
			"username": u.Username, "password": u.Password, "data_limit": u.DataLimit,
			"data_used": u.DataUsed, "expiry_unix": u.ExpiryDate.Unix(),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(syncList)
}

func handleApiUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !checkNodeToken(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var payload UsagePayload
	json.NewDecoder(r.Body).Decode(&payload)
	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP == "" {
		clientIP, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	database.IncrementUserDataUsed(payload.Username, payload.BytesAdded, strings.Split(clientIP, ",")[0])
}

func handleApiOnline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !checkNodeToken(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP == "" {
		clientIP, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	clientIP = strings.Split(clientIP, ",")[0]
	now := time.Now().Unix()
	database.UpdateNodeLastSeen(clientIP, now)
	var onlineUsers []string
	if err := json.NewDecoder(r.Body).Decode(&onlineUsers); err == nil {
		sshvpn.UpdateNodeOnlineStatus(onlineUsers)
		for _, u := range onlineUsers {
			database.UpdateLastSeen(u, now)
			database.IncrementUserDataUsed(u, 0, clientIP)
		}
	}
}

func handleApiInternalOnline(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host != "127.0.0.1" && host != "::1" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sshvpn.GetOnlineUsersList())
}

func handleActions(w http.ResponseWriter, r *http.Request) {
	if !checkAdminAuth(r) || r.Method != http.MethodPost {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	r.ParseForm()
	action := r.FormValue("action")
	successMsg := ""
	failMsg := ""

	switch action {
	case "create_user":
		vol, _ := strconv.ParseFloat(r.FormValue("volume"), 64)
		days, _ := strconv.Atoi(r.FormValue("days"))
		udpgw, _ := strconv.Atoi(r.FormValue("udpgw"))
		if udpgw == 0 {
			udpgw = 7301
		}
		if _, err := database.CreateUser(r.FormValue("username"), r.FormValue("password"), days, vol, udpgw, r.FormValue("comment")); err != nil {
			failMsg = "user_create_failed"
		} else {
			successMsg = "user_created"
		}
	case "full_edit_user":
		username := r.FormValue("username")
		var editErr error
		if p := r.FormValue("password"); p != "" {
			_, editErr = database.DB.Exec("UPDATE users SET password = ? WHERE username = ?", p, username)
		}
		if editErr == nil {
			if d := r.FormValue("days"); d != "" {
				days, _ := strconv.Atoi(d)
				_, editErr = database.DB.Exec("UPDATE users SET expiry_date = ? WHERE username = ?", time.Now().AddDate(0, 0, days), username)
			}
		}
		if editErr == nil {
			if v := r.FormValue("volume"); v != "" {
				vol, _ := strconv.ParseFloat(v, 64)
				_, editErr = database.DB.Exec("UPDATE users SET data_limit = ? WHERE username = ?", int64(vol*float64(models.GB)), username)
			}
		}
		if editErr == nil {
			if u := r.FormValue("udpgw"); u != "" {
				udpgw, _ := strconv.Atoi(u)
				_, editErr = database.DB.Exec("UPDATE users SET udpgw_port = ? WHERE username = ?", udpgw, username)
			}
		}
		if editErr == nil {
			_, editErr = database.DB.Exec("UPDATE users SET comment = ? WHERE username = ?", r.FormValue("comment"), username)
		}
		if editErr != nil {
			failMsg = "user_edit_failed"
		} else {
			successMsg = "user_edited"
		}
	case "reset_traffic":
		if err := database.ResetUserDataUsed(r.FormValue("username")); err != nil {
			failMsg = "user_reset_failed"
		} else {
			successMsg = "user_reset"
		}
	case "delete_user":
		if err := database.DeleteUser(r.FormValue("username")); err != nil {
			failMsg = "user_delete_failed"
		} else {
			successMsg = "user_deleted"
		}
	case "edit_node":
		if err := database.UpdateNodeSettings(r.FormValue("ip"), r.FormValue("domain"), r.FormValue("remark")); err != nil {
			failMsg = "node_edit_failed"
		} else {
			successMsg = "node_edited"
		}
	case "delete_node":
		if err := database.DeleteNode(r.FormValue("ip")); err != nil {
			failMsg = "node_delete_failed"
		} else {
			successMsg = "node_deleted"
		}
	case "update_settings":
		var setErr error
		for _, kv := range []struct{ k, v string }{
			{"announcement_url", r.FormValue("announcement_url")},
			{"tutorial_url", r.FormValue("tutorial_url")},
			{"auto_backup_hours", r.FormValue("auto_backup_hours")},
			{"tg_bot_token", r.FormValue("tg_bot_token")},
			{"tg_chat_id", r.FormValue("tg_chat_id")},
		} {
			if err := database.SetSetting(kv.k, kv.v); err != nil {
				setErr = err
			}
		}
		if setErr != nil {
			failMsg = "settings_save_failed"
		} else {
			successMsg = "settings_saved"
		}
	case "change_credentials":
		var credErr error
		if u := r.FormValue("admin_username"); u != "" {
			credErr = database.SetSetting("admin_username", u)
		}
		if credErr == nil {
			if p := r.FormValue("admin_password"); p != "" {
				credErr = database.SetAdminPassword(p)
			}
		}
		if credErr != nil {
			failMsg = "credentials_change_failed"
		} else {
			successMsg = "credentials_changed"
		}
	}

	url := "/admin/dashboard?tab=" + r.FormValue("current_tab")
	if failMsg != "" {
		url += "&error=" + failMsg
	} else if successMsg != "" {
		url += "&success=" + successMsg
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		dbUser := database.GetAdminUsername()
		userOK := subtle.ConstantTimeCompare([]byte(r.FormValue("username")), []byte(dbUser)) == 1
		passOK := database.VerifyAdminPassword(r.FormValue("password"))
		if userOK && passOK {
			setSessionCookie(w, r, newSessionToken())
			http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/admin/login?error=true", http.StatusSeeOther)
		return
	}
	fmt.Fprint(w, renderLoginHTML())
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		destroySession(cookie.Value)
	}
	clearSessionCookie(w, r)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	if !checkAdminAuth(r) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	nodes, _ := database.GetNodes()
	nodeName := func(ip string) string {
		for _, n := range nodes {
			if n.IP == ip && n.CustomRemark != "" {
				return n.CustomRemark
			}
		}
		return ip
	}
	chartJSON, _ := json.Marshal(buildClusterChartRaw(nodeName))

	currentTab := r.URL.Query().Get("tab")
	if currentTab == "" {
		currentTab = "dashboard"
	}

	toastMsg := ""
	toastIsError := false
	switch r.URL.Query().Get("success") {
	case "user_created":
		toastMsg = "\u2705 \u06a9\u0627\u0631\u0628\u0631 \u062c\u062f\u06cc\u062f \u0633\u0627\u062e\u062a\u0647 \u0634\u062f!"
	case "user_edited":
		toastMsg = "\u2705 \u06a9\u0627\u0631\u0628\u0631 \u0648\u06cc\u0631\u0627\u06cc\u0634 \u0634\u062f!"
	case "user_reset":
		toastMsg = "\U0001F504 \u062a\u0631\u0627\u0641\u06cc\u06a9 \u0635\u0641\u0631 \u0634\u062f!"
	case "user_deleted":
		toastMsg = "\U0001F5D1\uFE0F \u06a9\u0627\u0631\u0628\u0631 \u062d\u0630\u0641 \u0634\u062f!"
	case "node_edited":
		toastMsg = "\u2705 \u0646\u0648\u062f \u0628\u0631\u0648\u0632\u0631\u0633\u0627\u0646\u06cc \u0634\u062f!"
	case "node_deleted":
		toastMsg = "🗑️ نود حذف شد!"
	case "settings_saved":
		toastMsg = "\U0001F4BE \u062a\u0646\u0638\u06cc\u0645\u0627\u062a \u0630\u062e\u06cc\u0631\u0647 \u0634\u062f!"
	case "credentials_changed":
		toastMsg = "\U0001F510 \u0627\u0637\u0644\u0627\u0639\u0627\u062a \u0648\u0631\u0648\u062f \u062a\u063a\u06cc\u06cc\u0631 \u06a9\u0631\u062f!"
	case "backup_restored":
		toastMsg = "\u2705 \u0628\u06a9\u0627\u067e \u0628\u0627 \u0645\u0648\u0641\u0642\u06cc\u062a \u0628\u0627\u0632\u06af\u0631\u062f\u0627\u0646\u06cc \u0634\u062f!"
	}
	if toastMsg == "" {
		switch r.URL.Query().Get("error") {
		case "user_create_failed":
			toastMsg = "\u274C \u062e\u0637\u0627 \u062f\u0631 \u0633\u0627\u062e\u062a \u06a9\u0627\u0631\u0628\u0631 (\u0646\u0627\u0645 \u062a\u06a9\u0631\u0627\u0631\u06cc \u06cc\u0627 \u062e\u0637\u0627\u06cc \u062f\u06cc\u062a\u0627\u0628\u06cc\u0633)"
			toastIsError = true
		case "user_edit_failed":
			toastMsg = "\u274C \u062e\u0637\u0627 \u062f\u0631 \u0648\u06cc\u0631\u0627\u06cc\u0634 \u06a9\u0627\u0631\u0628\u0631"
			toastIsError = true
		case "user_reset_failed":
			toastMsg = "\u274C \u062e\u0637\u0627 \u062f\u0631 \u0631\u06cc\u0633\u062a \u062a\u0631\u0627\u0641\u06cc\u06a9"
			toastIsError = true
		case "user_delete_failed":
			toastMsg = "\u274C \u062e\u0637\u0627 \u062f\u0631 \u062d\u0630\u0641 \u06a9\u0627\u0631\u0628\u0631"
			toastIsError = true
		case "node_edit_failed":
			toastMsg = "\u274C \u062e\u0637\u0627 \u062f\u0631 \u0628\u0631\u0648\u0632\u0631\u0633\u0627\u0646\u06cc \u0646\u0648\u062f"
			toastIsError = true
		case "node_delete_failed":
			toastMsg = "❌ خطا در حذف نود"
			toastIsError = true
		case "settings_save_failed":
			toastMsg = "\u274C \u062e\u0637\u0627 \u062f\u0631 \u0630\u062e\u06cc\u0631\u0647 \u062a\u0646\u0638\u06cc\u0645\u0627\u062a"
			toastIsError = true
		case "credentials_change_failed":
			toastMsg = "\u274C \u062e\u0637\u0627 \u062f\u0631 \u062a\u063a\u06cc\u06cc\u0631 \u0627\u0637\u0644\u0627\u0639\u0627\u062a \u0648\u0631\u0648\u062f"
			toastIsError = true
		case "backup_restore_failed":
			toastMsg = "\u274C \u062e\u0637\u0627 \u062f\u0631 \u0628\u0627\u0632\u06af\u0631\u062f\u0627\u0646\u06cc \u0628\u06a9\u0627\u067e (\u0641\u0627\u06cc\u0644 \u0646\u0627\u0645\u0639\u062a\u0628\u0631 \u06cc\u0627 \u062e\u0637\u0627\u06cc \u062f\u06cc\u062a\u0627\u0628\u06cc\u0633)"
			toastIsError = true
		}
	}

	backupText, backupColor := "\u063a\u06cc\u0631\u0641\u0639\u0627\u0644", "bg-red-500/20 text-red-400 border-red-500/30"
	if database.GetSetting("auto_backup_hours") != "" && database.GetSetting("tg_bot_token") != "" {
		if database.GetSetting("last_backup_status") == "FAILED" {
			backupText, backupColor = "\u062e\u0637\u0627 \u062f\u0631 \u0627\u0631\u0633\u0627\u0644", "bg-amber-500/20 text-amber-400 border-amber-500/30"
		} else {
			backupText, backupColor = "\u0641\u0639\u0627\u0644", "bg-emerald-500/20 text-emerald-400 border-emerald-500/30"
		}
	}

	logContent := "\u0647\u06cc\u0686 \u0644\u0627\u06af\u06cc \u062b\u0628\u062a \u0646\u0634\u062f\u0647 \u0627\u0633\u062a."
	if logs, err := os.ReadFile("/root/svm-panel/system.log"); err == nil {
		logContent = html.EscapeString(string(logs))
	}

	adminUser := database.GetAdminUsername()

	if isMobile(r) {
		fmt.Fprint(w, renderMobileDashboard(currentTab, toastMsg, toastIsError, string(chartJSON), backupColor, backupText, logContent, GlobalToken, adminUser))
	} else {
		fmt.Fprint(w, renderDesktopDashboard(currentTab, toastMsg, toastIsError, string(chartJSON), backupColor, backupText, logContent, GlobalToken, adminUser))
	}
}

func handleSub(w http.ResponseWriter, r *http.Request) {
	subToken := strings.TrimPrefix(r.URL.Path, "/sub/")
	if subToken == "" {
		http.Error(w, "404 - \u0644\u06cc\u0646\u06a9 \u0646\u0627\u0645\u0639\u062a\u0628\u0631 \u0627\u0633\u062a", http.StatusNotFound)
		return
	}

	user, err := database.GetUserBySubToken(subToken)
	if err != nil {
		http.Error(w, "404 - \u06a9\u0627\u0631\u0628\u0631 \u06cc\u0627\u0641\u062a \u0646\u0634\u062f", http.StatusNotFound)
		return
	}

	iranTime := models.IranTime
	expiry := user.ExpiryDate.In(iranTime)

	statusBadge := "<span style=\"background: #10B981; color: white; padding: 5px 12px; border-radius: 20px; font-size: 14px;\">\u2714 \u0641\u0639\u0627\u0644</span>"
	if user.IsExpired() || user.IsOverLimit() {
		statusBadge = "<span style=\"background: #EF4444; color: white; padding: 5px 12px; border-radius: 20px; font-size: 14px;\">\u2716 \u063a\u06cc\u0631\u0641\u0639\u0627\u0644</span>"
	}

	onlineBadge := "<span style=\"color: #EF4444; font-weight: bold;\">\U0001F534 \u0622\u0641\u0644\u0627\u06cc\u0646</span>"
	if sshvpn.IsUserOnline(user.Username) {
		onlineBadge = "<span style=\"color: #10B981; font-weight: bold;\">\U0001F7E2 \u0622\u0646\u0644\u0627\u06cc\u0646</span>"
	}

	lastSeen := "\u0628\u062f\u0648\u0646 \u0627\u062a\u0635\u0627\u0644"
	if user.LastSeen > 0 {
		lastSeen = time.Unix(user.LastSeen, 0).In(iranTime).Format("2006-01-02 15:04")
	}

	unlimited := user.DataLimit == 0
	percent := 0.0
	barColor := "#10B981"
	limitFmt := "\u0646\u0627\u0645\u062d\u062f\u0648\u062f"
	remFmt := "\u0646\u0627\u0645\u062d\u062f\u0648\u062f"
	if !unlimited {
		percent = float64(user.DataUsed) / float64(user.DataLimit) * 100
		if percent > 100 {
			percent = 100
		}
		if percent > 80 {
			barColor = "#EF4444"
		} else if percent > 50 {
			barColor = "#F59E0B"
		}
		limitFmt = formatBytes(user.DataLimit)
		rem := user.DataLimit - user.DataUsed
		if rem < 0 {
			rem = 0
		}
		remFmt = formatBytes(rem)
	}

	nodes, _ := database.GetNodes()
	var configHTML strings.Builder
	configHTML.WriteString("<div class=\"info-box\"><h3 style=\"margin-bottom:15px;\">\U0001F4CB \u06a9\u0627\u0646\u0641\u06cc\u06af\u200c\u0647\u0627\u06cc \u0627\u062a\u0635\u0627\u0644 (NPV)</h3>")

	serverHost := strings.Split(r.Host, ":")[0]
	genNPV := func(rem, host string) string {
		p := user.UdpgwPort
		if p == 0 {
			p = 7301
		}
		b, _ := json.Marshal(NPVConfig{SSHConfigType: "SSH-Direct", Remarks: rem, SSHHost: host, SSHPort: 2222, SSHUsername: user.Username, SSHPassword: user.Password, UDPGWPort: p, UDPGWTransparentDNS: true})
		return "npvt-ssh://" + base64.StdEncoding.EncodeToString(b)
	}

	mainHost, mainRem := serverHost, "Local S1"
	for _, n := range nodes {
		if n.IP == "Main-Server" {
			if n.Domain != "" {
				mainHost = n.Domain
			}
			if n.CustomRemark != "" {
				mainRem = n.CustomRemark
			} else {
				c, f := getCountryFlag(serverHost)
				mainRem = f + " " + c + " S1"
			}
			break
		}
	}
	configHTML.WriteString(fmt.Sprintf("<div class=\"config-item\"><span>%s</span><button class=\"config-btn\" onclick=\"copySingle('%s')\">\u06a9\u067e\u06cc</button></div>", html.EscapeString(mainRem), html.EscapeString(genNPV(mainRem, mainHost))))

	idx := 2
	for _, n := range nodes {
		if n.IP != "Main-Server" && n.IP != "" && n.IP != "127.0.0.1" {
			h, rem := n.IP, n.CustomRemark
			if n.Domain != "" {
				h = n.Domain
			}
			if rem == "" {
				c, f := getCountryFlag(n.IP)
				rem = fmt.Sprintf("%s %s S%d", f, c, idx)
			}
			configHTML.WriteString(fmt.Sprintf("<div class=\"config-item\"><span>%s</span><button class=\"config-btn\" onclick=\"copySingle('%s')\">\u06a9\u067e\u06cc</button></div>", html.EscapeString(rem), html.EscapeString(genNPV(rem, h))))
			idx++
		}
	}
	configHTML.WriteString(`</div>`)

	subRaw := buildSubChartRaw(user.Username, nodes, mainRem)
	subRawBytes, _ := json.Marshal(subRaw)

	fmt.Fprint(w, renderSubHTML(user.Username, barColor, percent, statusBadge, onlineBadge, limitFmt, formatBytes(user.DataUsed), remFmt, expiry.Format("2006-01-02 15:04"), lastSeen, database.GetSetting("announcement_url"), database.GetSetting("tutorial_url"), configHTML.String(), string(subRawBytes)))
}

func handleChartFull(w http.ResponseWriter, r *http.Request) {
	if !checkAdminAuth(r) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	nodes, _ := database.GetNodes()
	nodeName := func(ip string) string {
		for _, n := range nodes {
			if n.IP == ip && n.CustomRemark != "" {
				return n.CustomRemark
			}
		}
		return ip
	}
	chartJSON, _ := json.Marshal(buildClusterChartRaw(nodeName))
	fmt.Fprint(w, renderFullChartHTML(string(chartJSON)))
}

// ---------------------------------------------------------------------------
// Raw chart builders (hourly + daily) shared by dashboard, node chart, sub
// page and full-screen page. The client slices the window and computes the
// total + trend, so changing the range needs no extra server round-trip.
// ---------------------------------------------------------------------------

type chartSeriesRaw struct {
	Name string    `json:"name"`
	Data []float64 `json:"data"`
}

type chartPartRaw struct {
	Categories []string         `json:"categories"`
	Series     []chartSeriesRaw `json:"series"`
}

type chartRawData struct {
	Hourly chartPartRaw `json:"hourly"`
	Daily  chartPartRaw `json:"daily"`
}

func buildClusterChartRaw(nodeName func(string) string) chartRawData {
	loc := models.IranTime
	now := time.Now().In(loc)

	hourlyCats := make([]string, 24)
	for i := 0; i < 24; i++ {
		hourlyCats[i] = now.Add(-time.Duration(23-i) * time.Hour).Format("15:00")
	}
	hourlyMap := map[string][]float64{}
	startHour := now.Add(-23 * time.Hour).Truncate(time.Hour)
	if rows, err := database.DB.Query(`
		SELECT node_ip, DATE_FORMAT(log_date,'%Y-%m-%d') AS d, log_hour, SUM(bytes_used)
		FROM traffic_logs
		WHERE TIMESTAMP(log_date, MAKETIME(log_hour,0,0)) >= ?
		GROUP BY node_ip, d, log_hour`, startHour.Format("2006-01-02 15:00:00")); err == nil {
		truncNow := now.Truncate(time.Hour)
		for rows.Next() {
			var ip, d string
			var h int
			var b int64
			if rows.Scan(&ip, &d, &h, &b) != nil {
				continue
			}
			t, perr := parseLogDate(d, loc)
			if perr != nil {
				continue
			}
			t = t.Add(time.Duration(h) * time.Hour)
			idx := 23 - int(truncNow.Sub(t).Hours())
			if idx < 0 || idx > 23 {
				continue
			}
			nm := nodeName(ip)
			if _, ok := hourlyMap[nm]; !ok {
				hourlyMap[nm] = make([]float64, 24)
			}
			hourlyMap[nm][idx] += float64(b) / float64(models.GB)
		}
		rows.Close()
	}

	var dailyCats []string
	dayIndex := map[string]int{}
	type dk struct {
		ip, d string
	}
	agg := map[dk]float64{}
	if rows, err := database.DB.Query(`
		SELECT node_ip, DATE_FORMAT(log_date,'%Y-%m-%d') AS d, SUM(bytes_used)
		FROM traffic_logs
		GROUP BY node_ip, d
		ORDER BY d ASC`); err == nil {
		for rows.Next() {
			var ip, d string
			var b int64
			if rows.Scan(&ip, &d, &b) != nil {
				continue
			}
			if _, ok := dayIndex[d]; !ok {
				dayIndex[d] = len(dailyCats)
				if t, perr := parseLogDate(d, loc); perr == nil {
					dailyCats = append(dailyCats, t.Format("01/02"))
				} else {
					dailyCats = append(dailyCats, d)
				}
			}
			agg[dk{ip, d}] = float64(b) / float64(models.GB)
		}
		rows.Close()
	}
	dailyMap := map[string][]float64{}
	for k, v := range agg {
		nm := nodeName(k.ip)
		if _, ok := dailyMap[nm]; !ok {
			dailyMap[nm] = make([]float64, len(dailyCats))
		}
		dailyMap[nm][dayIndex[k.d]] = v
	}

	toSeries := func(m map[string][]float64, n int) []chartSeriesRaw {
		names := make([]string, 0, len(m))
		for nm := range m {
			names = append(names, nm)
		}
		sort.Strings(names)
		var out []chartSeriesRaw
		for _, nm := range names {
			out = append(out, chartSeriesRaw{Name: nm, Data: m[nm]})
		}
		if len(out) == 0 {
			out = append(out, chartSeriesRaw{Name: "\u0628\u062f\u0648\u0646 \u062a\u0631\u0627\u0641\u06cc\u06a9", Data: make([]float64, n)})
		}
		return out
	}

	return chartRawData{
		Hourly: chartPartRaw{Categories: hourlyCats, Series: toSeries(hourlyMap, 24)},
		Daily:  chartPartRaw{Categories: dailyCats, Series: toSeries(dailyMap, len(dailyCats))},
	}
}

// buildNodeChartRaw returns hourly (24 buckets) + daily (all days) raw data for
// a single node, as a single series named after the node.
func buildNodeChartRaw(ip, name string) chartRawData {
	loc := models.IranTime
	now := time.Now().In(loc)

	hourlyCats := make([]string, 24)
	for i := 0; i < 24; i++ {
		hourlyCats[i] = now.Add(-time.Duration(23-i) * time.Hour).Format("15:00")
	}
	hourlyData := make([]float64, 24)
	startHourStr := now.Add(-23 * time.Hour).Truncate(time.Hour).Format("2006-01-02 15:00:00")
	if rows, err := database.DB.Query(`
		SELECT DATE_FORMAT(log_date,'%Y-%m-%d') d, log_hour, SUM(bytes_used)
		FROM node_traffic_logs
		WHERE ip = ? AND TIMESTAMP(log_date, MAKETIME(log_hour,0,0)) >= ?
		GROUP BY d, log_hour`, ip, startHourStr); err == nil {
		truncNow := now.Truncate(time.Hour)
		for rows.Next() {
			var d string
			var h int
			var b int64
			if rows.Scan(&d, &h, &b) != nil {
				continue
			}
			t, perr := parseLogDate(d, loc)
			if perr != nil {
				continue
			}
			t = t.Add(time.Duration(h) * time.Hour)
			idx := 23 - int(truncNow.Sub(t).Hours())
			if idx >= 0 && idx <= 23 {
				hourlyData[idx] += float64(b) / float64(models.GB)
			}
		}
		rows.Close()
	}

	var dailyCats []string
	dayIndex := map[string]int{}
	agg := map[string]float64{}
	if rows, err := database.DB.Query(`
		SELECT DATE_FORMAT(log_date,'%Y-%m-%d') d, SUM(bytes_used)
		FROM node_traffic_logs
		WHERE ip = ?
		GROUP BY d
		ORDER BY d ASC`, ip); err == nil {
		for rows.Next() {
			var d string
			var b int64
			if rows.Scan(&d, &b) != nil {
				continue
			}
			if _, ok := dayIndex[d]; !ok {
				dayIndex[d] = len(dailyCats)
				if t, perr := parseLogDate(d, loc); perr == nil {
					dailyCats = append(dailyCats, t.Format("01/02"))
				} else {
					dailyCats = append(dailyCats, d)
				}
			}
			agg[d] = float64(b) / float64(models.GB)
		}
		rows.Close()
	}
	dailyData := make([]float64, len(dailyCats))
	for d, v := range agg {
		dailyData[dayIndex[d]] = v
	}

	return chartRawData{
		Hourly: chartPartRaw{Categories: hourlyCats, Series: []chartSeriesRaw{{Name: name, Data: hourlyData}}},
		Daily:  chartPartRaw{Categories: dailyCats, Series: []chartSeriesRaw{{Name: name, Data: dailyData}}},
	}
}

// buildSubChartRaw returns hourly (24 buckets) + daily (all days the user had
// traffic) raw data, one stacked series per node the user actually used.
func buildSubChartRaw(username string, nodes []database.Node, mainRem string) chartRawData {
	loc := models.IranTime
	now := time.Now().In(loc)

	labelOf := func(ip string) string {
		if ip == "Main-Server" {
			return mainRem
		}
		for _, n := range nodes {
			if n.IP == ip && n.CustomRemark != "" {
				return n.CustomRemark
			}
		}
		return ip
	}

	var userNodes []string
	if rows, err := database.DB.Query("SELECT DISTINCT node_ip FROM traffic_logs WHERE username = ?", username); err == nil {
		for rows.Next() {
			var nip string
			if rows.Scan(&nip) == nil {
				userNodes = append(userNodes, nip)
			}
		}
		rows.Close()
	}
	if len(userNodes) == 0 {
		userNodes = []string{"Main-Server"}
	}

	hourlyCats := make([]string, 24)
	for i := 0; i < 24; i++ {
		hourlyCats[i] = now.Add(-time.Duration(23-i) * time.Hour).Format("15:00")
	}
	startHourStr := now.Add(-23 * time.Hour).Truncate(time.Hour).Format("2006-01-02 15:00:00")
	var hourlySeries []chartSeriesRaw
	for _, ip := range userNodes {
		data := make([]float64, 24)
		if rows, err := database.DB.Query(`
			SELECT DATE_FORMAT(log_date,'%Y-%m-%d') d, log_hour, SUM(bytes_used)
			FROM traffic_logs
			WHERE username = ? AND node_ip = ? AND TIMESTAMP(log_date, MAKETIME(log_hour,0,0)) >= ?
			GROUP BY d, log_hour`, username, ip, startHourStr); err == nil {
			truncNow := now.Truncate(time.Hour)
			for rows.Next() {
				var d string
				var h int
				var b int64
				if rows.Scan(&d, &h, &b) != nil {
					continue
				}
				t, perr := parseLogDate(d, loc)
				if perr != nil {
					continue
				}
				t = t.Add(time.Duration(h) * time.Hour)
				idx := 23 - int(truncNow.Sub(t).Hours())
				if idx >= 0 && idx <= 23 {
					data[idx] += float64(b) / float64(models.GB)
				}
			}
			rows.Close()
		}
		hourlySeries = append(hourlySeries, chartSeriesRaw{Name: labelOf(ip), Data: data})
	}

	var dailyCats []string
	dayIndex := map[string]int{}
	if rows, err := database.DB.Query(`
		SELECT DISTINCT DATE_FORMAT(log_date,'%Y-%m-%d') d
		FROM traffic_logs
		WHERE username = ?
		ORDER BY d ASC`, username); err == nil {
		for rows.Next() {
			var d string
			if rows.Scan(&d) != nil {
				continue
			}
			dayIndex[d] = len(dailyCats)
			if t, perr := parseLogDate(d, loc); perr == nil {
				dailyCats = append(dailyCats, t.Format("01/02"))
			} else {
				dailyCats = append(dailyCats, d)
			}
		}
		rows.Close()
	}
	var dailySeries []chartSeriesRaw
	for _, ip := range userNodes {
		data := make([]float64, len(dailyCats))
		if rows, err := database.DB.Query(`
			SELECT DATE_FORMAT(log_date,'%Y-%m-%d') d, SUM(bytes_used)
			FROM traffic_logs
			WHERE username = ? AND node_ip = ?
			GROUP BY d`, username, ip); err == nil {
			for rows.Next() {
				var d string
				var b int64
				if rows.Scan(&d, &b) != nil {
					continue
				}
				if j, ok := dayIndex[d]; ok {
					data[j] = float64(b) / float64(models.GB)
				}
			}
			rows.Close()
		}
		dailySeries = append(dailySeries, chartSeriesRaw{Name: labelOf(ip), Data: data})
	}

	return chartRawData{
		Hourly: chartPartRaw{Categories: hourlyCats, Series: hourlySeries},
		Daily:  chartPartRaw{Categories: dailyCats, Series: dailySeries},
	}
}
// handleStatic serves files from a static directory (default /root/svm-panel/static,
// overridable via SVM_STATIC_DIR). Used for the panel logo and similar assets so
// the browser can cache them. Path traversal is blocked.
func handleStatic(w http.ResponseWriter, r *http.Request) {
	dir := os.Getenv("SVM_STATIC_DIR")
	if dir == "" {
		dir = "/root/svm-panel/static"
	}
	rel := strings.TrimPrefix(r.URL.Path, "/static/")
	if rel == "" || strings.Contains(rel, "..") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, dir+"/"+rel)
}
