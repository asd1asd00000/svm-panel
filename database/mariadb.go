package database

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"

	"github.com/asd1asd00000/svm-panel/models"
)

var DB *sql.DB

// Re-export the shared domain models so existing callers that referenced
// database.User / database.Node / database.TrafficStats keep working.
type (
	User         = models.User
	Node         = models.Node
	TrafficStats = models.TrafficStats
)

// logPath is where the panel writes its system log. Overridable via env.
func logPath() string {
	if p := os.Getenv("SVM_LOG_PATH"); p != "" {
		return p
	}
	return "/root/svm-panel/system.log"
}

// dsn builds the MariaDB connection string. Credentials can be overridden with
// the SVM_DB_DSN environment variable instead of the hard-coded root default.
func dsn(withDB bool) string {
	if v := os.Getenv("SVM_DB_DSN"); v != "" {
		return v
	}
	if withDB {
		return "root:@tcp(127.0.0.1:3306)/svm_db?parseTime=true"
	}
	return "root:@tcp(127.0.0.1:3306)/?parseTime=true"
}

// ---------------------------------------------------------------------------
// DB credentials for shell tools (mysqldump / mysql).
//
// The Go connection uses SVM_DB_DSN directly, but the exec-based backup/restore
// calls need the same credentials handed to the mysql CLI. We never hard-code
// "-u root" with no password (that breaks as soon as the DB root is secured),
// and we never put the password on the command line (it would leak via ps).
// Instead we read the credentials and write them to a short-lived, 0600
// defaults file passed via --defaults-extra-file.
// ---------------------------------------------------------------------------

// parseDSNCredentials extracts user and password from a go-sql-driver DSN.
// It is robust for the common shapes used by this panel (e.g.
// "root:pass@tcp(127.0.0.1:3306)/svm_db?parseTime=true"). If the password
// itself contains the literal substring "@tcp(" / "@unix(" / "@/" it may be
// mis-parsed; in that case set SVM_DB_ROOT_PASS explicitly instead.
func parseDSNCredentials(d string) (user, pass string) {
	if i := strings.Index(d, "?"); i >= 0 {
		d = d[:i] // drop query string
	}
	at := -1
	for _, marker := range []string{"@tcp(", "@unix(", "@/"} {
		if j := strings.Index(d, marker); j >= 0 {
			at = j
			break
		}
	}
	if at < 0 {
		at = strings.LastIndex(d, "@")
		if at < 0 {
			return "", ""
		}
	}
	creds := d[:at]
	if c := strings.Index(creds, ":"); c >= 0 {
		return creds[:c], creds[c+1:]
	}
	return creds, ""
}

// dbCredentials returns the user/password the mysql CLI should use.
// Priority: SVM_DB_ROOT_PASS (explicit override) > parsed from SVM_DB_DSN > none.
func dbCredentials() (user, pass string) {
	user = "root"
	if v := os.Getenv("SVM_DB_ROOT_PASS"); v != "" {
		if d := os.Getenv("SVM_DB_DSN"); d != "" {
			if u, _ := parseDSNCredentials(d); u != "" {
				user = u
			}
		}
		return user, v
	}
	d := os.Getenv("SVM_DB_DSN")
	if d == "" {
		return user, "" // default install: root with no password
	}
	u, p := parseDSNCredentials(d)
	if u != "" {
		user = u
	}
	return user, p
}

// writeMySQLDefaultsFile writes a temporary [client] option file (mode 0600)
// holding the given credentials and returns its path plus a cleanup func.
// The caller must defer the cleanup. The password is quoted so values with
// spaces or special characters survive; embedded double-quotes are escaped.
func writeMySQLDefaultsFile(user, pass string) (string, func(), error) {
	f, err := os.CreateTemp("", "svm-my-*.cnf")
	if err != nil {
		return "", nil, err
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	_ = f.Chmod(0600)

	var b strings.Builder
	b.WriteString("[client]\n")
	b.WriteString("user=" + user + "\n")
	if pass != "" {
		b.WriteString("password=\"" + strings.ReplaceAll(pass, "\"", "\\\"") + "\"\n")
	}
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		cleanup()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

func ConnectDB() {
	tempDB, err := sql.Open("mysql", dsn(false))
	if err != nil {
		log.Fatalf("Error connecting to MySQL for DB creation: %v\n", err)
	}
	_, err = tempDB.Exec("CREATE DATABASE IF NOT EXISTS svm_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	if err != nil {
		log.Fatalf("Error creating database: %v\n", err)
	}
	tempDB.Close()

	DB, err = sql.Open("mysql", dsn(true))
	if err != nil {
		log.Fatalf("Error connecting to database server: %v\n", err)
	}

	DB.SetConnMaxLifetime(3 * time.Minute)
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(25)

	if err = DB.Ping(); err != nil {
		log.Fatalf("Database server is unreachable: %v\n", err)
	}

	tables := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INT AUTO_INCREMENT PRIMARY KEY,
			username VARCHAR(255) UNIQUE NOT NULL,
			password VARCHAR(255) NOT NULL,
			expiry_date DATETIME,
			data_limit BIGINT DEFAULT 0,
			data_used BIGINT DEFAULT 0,
			last_seen BIGINT DEFAULT 0,
			sub_token VARCHAR(64),
			udpgw_port INT DEFAULT 7301,
			comment TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS traffic_logs (
			username VARCHAR(50) NOT NULL,
			log_date DATE NOT NULL,
			log_hour INT NOT NULL,
			node_ip VARCHAR(50) NOT NULL DEFAULT 'Main-Server',
			bytes_used BIGINT DEFAULT 0,
			PRIMARY KEY (username, log_date, log_hour, node_ip)
		);`,
		`CREATE TABLE IF NOT EXISTS nodes (
			ip VARCHAR(50) PRIMARY KEY,
			last_seen BIGINT DEFAULT 0,
			total_traffic BIGINT DEFAULT 0,
			name VARCHAR(50) DEFAULT '',
			domain VARCHAR(255) DEFAULT '',
			custom_remark VARCHAR(150) DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS node_traffic_logs (
			ip VARCHAR(50) NOT NULL,
			log_date DATE NOT NULL,
			log_hour INT NOT NULL,
			bytes_used BIGINT DEFAULT 0,
			PRIMARY KEY (ip, log_date, log_hour)
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key_name VARCHAR(255) PRIMARY KEY,
			key_value TEXT
		);`,
	}

	for _, query := range tables {
		if _, err = DB.Exec(query); err != nil {
			log.Fatalf("Error initializing table: %v\n", err)
		}
	}

	// Backwards-compatible column add for databases created before "comment"
	// existed. The error is ignored on purpose (column already there).
	_, _ = DB.Exec("ALTER TABLE users ADD COLUMN comment TEXT")
}

func generateSubToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

func SetSetting(key, value string) error {
	_, err := DB.Exec("REPLACE INTO settings (key_name, key_value) VALUES (?, ?)", key, value)
	return err
}

func GetSetting(key string) string {
	var val string
	if err := DB.QueryRow("SELECT key_value FROM settings WHERE key_name = ?", key).Scan(&val); err != nil {
		return ""
	}
	return val
}

// ---------------------------------------------------------------------------
// Admin credentials (bcrypt with transparent legacy-plaintext upgrade)
// ---------------------------------------------------------------------------

// GetAdminUsername returns the configured admin username or the default.
func GetAdminUsername() string {
	if u := GetSetting("admin_username"); u != "" {
		return u
	}
	return "admin"
}

// SetAdminPassword stores the admin password as a bcrypt hash.
func SetAdminPassword(plain string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return SetSetting("admin_password", string(hash))
}

// VerifyAdminPassword checks a login attempt. It supports both bcrypt hashes
// and legacy plaintext values written by older installers; when a legacy
// plaintext password matches it is transparently re-hashed with bcrypt.
func VerifyAdminPassword(plain string) bool {
	stored := GetSetting("admin_password")
	if stored == "" {
		return false
	}
	if strings.HasPrefix(stored, "$2") { // bcrypt hash
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(plain)) == nil
	}
	// Legacy plaintext – constant-time compare, then upgrade to bcrypt.
	if subtle.ConstantTimeCompare([]byte(stored), []byte(plain)) == 1 {
		_ = SetAdminPassword(plain)
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Telegram + logging
// ---------------------------------------------------------------------------

func SendTelegramMessage(text string) {
	botToken := GetSetting("tg_bot_token")
	chatID := GetSetting("tg_chat_id")
	if botToken == "" || chatID == "" {
		return
	}
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	resp, err := http.PostForm(apiURL, url.Values{"chat_id": {chatID}, "text": {text}})
	if err == nil {
		resp.Body.Close()
	}
}

func WriteSystemLog(level, msg string) {
	f, err := os.OpenFile(logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		f.WriteString(time.Now().Format("2006-01-02 15:04:05") + " | [" + level + "] " + msg + "\n")
		f.Close()
	}
}

func StartLogMaintenanceDaemon() {
	cleanLogs()
	for {
		time.Sleep(6 * time.Hour)
		cleanLogs()
	}
}

func cleanLogs() {
	filePath := logPath()
	os.Remove("/root/svm-panel/backup.log")

	content, err := os.ReadFile(filePath)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	var retainedLogs []string
	now := time.Now()

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, " | ", 2)
		if len(parts) < 2 {
			continue
		}

		logTime, err := time.Parse("2006-01-02 15:04:05", parts[0])
		if err != nil {
			continue
		}

		ageHours := now.Sub(logTime).Hours()
		isError := strings.Contains(parts[1], "[ERROR]") || strings.Contains(parts[1], "FAILED")
		isSuccess := strings.Contains(parts[1], "[INFO]") || strings.Contains(parts[1], "SUCCESS")

		switch {
		case isSuccess && ageHours <= 24:
			retainedLogs = append(retainedLogs, line)
		case isError && ageHours <= (30*24):
			retainedLogs = append(retainedLogs, line)
		case !isSuccess && !isError && ageHours <= 24:
			retainedLogs = append(retainedLogs, line)
		}
	}

	output := strings.Join(retainedLogs, "\n")
	if len(retainedLogs) > 0 {
		output += "\n"
	}
	_ = os.WriteFile(filePath, []byte(output), 0644)
}

// ---------------------------------------------------------------------------
// Backup / restore (no shell interpolation -> no command injection;
// credentials via a temp defaults file -> no password on the command line)
// ---------------------------------------------------------------------------

func RunAdvancedBackup(zipPass string) error {
	botToken := GetSetting("tg_bot_token")
	chatID := GetSetting("tg_chat_id")
	timestamp := time.Now().Format("20060102_150405")
	sqlFile := fmt.Sprintf("/tmp/svm_backup_%s.sql", timestamp)
	zipFile := fmt.Sprintf("/root/svm_backup_%s.zip", timestamp)

	user, pass := dbCredentials()
	cnfPath, cleanupCnf, err := writeMySQLDefaultsFile(user, pass)
	if err != nil {
		return fmt.Errorf("prepare db credentials file: %w", err)
	}
	defer cleanupCnf()

	cmdDump := exec.Command("mysqldump", "--defaults-extra-file="+cnfPath, "svm_db")
	outFile, err := os.Create(sqlFile)
	if err != nil {
		return err
	}
	cmdDump.Stdout = outFile
	if err := cmdDump.Run(); err != nil {
		outFile.Close()
		return err
	}
	outFile.Close()
	defer os.Remove(sqlFile)

	var cmdZip *exec.Cmd
	if zipPass != "" {
		cmdZip = exec.Command("zip", "-P", zipPass, "-j", zipFile, sqlFile)
	} else {
		cmdZip = exec.Command("zip", "-j", zipFile, sqlFile)
	}
	if err := cmdZip.Run(); err != nil {
		return err
	}

	if botToken == "" || chatID == "" {
		return fmt.Errorf("telegram bot token or chat id is missing")
	}
	tgURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendDocument", botToken)
	out, err := exec.Command("curl", "-s", "-F", "chat_id="+chatID, "-F", "document=@"+zipFile, tgURL).CombinedOutput()
	if err != nil {
		return fmt.Errorf("curl failed: %v", err)
	}
	if !strings.Contains(string(out), "\"ok\":true") {
		return fmt.Errorf("telegram api error: %s", string(out))
	}
	return nil
}

// RestoreBackup unzips a MySQL dump and imports it. All arguments are passed as
// exec args (never through "sh -c"), so a malicious path/password cannot inject
// shell commands. DB credentials are supplied via a temp defaults file.
func RestoreBackup(zipPath, password string) error {
	args := []string{"-p"}
	if password != "" {
		args = append(args, "-P", password)
	}
	args = append(args, zipPath)

	unzip := exec.Command("unzip", args...)
	stdout, err := unzip.StdoutPipe()
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("/tmp", "restore-*.sql")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if err := unzip.Start(); err != nil {
		tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, stdout); err != nil {
		tmp.Close()
		return err
	}
	if err := unzip.Wait(); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		tmp.Close()
		return err
	}

	user, pass := dbCredentials()
	cnfPath, cleanupCnf, err := writeMySQLDefaultsFile(user, pass)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("prepare db credentials file: %w", err)
	}
	defer cleanupCnf()

	mysqlCmd := exec.Command("mysql", "--defaults-extra-file="+cnfPath, "svm_db")
	mysqlCmd.Stdin = tmp
	err = mysqlCmd.Run()
	tmp.Close()
	return err
}

func StartAutoBackupDaemon() {
	for {
		time.Sleep(1 * time.Minute)
		intervalStr := GetSetting("auto_backup_hours")
		interval, err := strconv.Atoi(intervalStr)
		if err != nil || interval <= 0 {
			continue
		}
		lastBackupStr := GetSetting("last_auto_backup_unix")
		lastBackup, _ := strconv.ParseInt(lastBackupStr, 10, 64)

		if time.Now().Unix()-lastBackup >= int64(interval*3600) {
			if err := RunAdvancedBackup(GetSetting("zip_password")); err != nil {
				SetSetting("last_backup_status", "FAILED")
				WriteSystemLog("ERROR", "Auto-Backup Failed: "+err.Error())
				SendTelegramMessage("\u274c SVM Panel Auto-Backup Failed!\nTime: " + time.Now().Format("2006-01-02 15:04:05") + "\nError: " + err.Error())
			} else {
				SetSetting("last_backup_status", "SUCCESS")
				WriteSystemLog("INFO", "Auto-Backup created and sent to Telegram successfully.")
			}
			_ = SetSetting("last_auto_backup_unix", fmt.Sprintf("%d", time.Now().Unix()))
		}
	}
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

func CreateUser(username, password string, days int, volumeGB float64, udpgwPort int, comment string) (string, error) {
	expiryDate := time.Now().AddDate(0, 0, days)
	dataLimit := int64(volumeGB * float64(models.GB))
	token := generateSubToken()
	_, err := DB.Exec(
		"INSERT INTO users (username, password, expiry_date, data_limit, data_used, udpgw_port, sub_token, comment) VALUES (?, ?, ?, ?, 0, ?, ?, ?)",
		username, password, expiryDate, dataLimit, udpgwPort, token, comment,
	)
	return token, err
}

func DeleteUser(username string) error {
	_, err := DB.Exec("DELETE FROM users WHERE username = ?", username)
	return err
}

func UpdateUserExpiry(username string, addDays int) error {
	_, err := DB.Exec("UPDATE users SET expiry_date = DATE_ADD(expiry_date, INTERVAL ? DAY) WHERE username = ?", addDays, username)
	return err
}

func UpdateUserDataLimit(username string, newVolumeGB float64) error {
	dataLimit := int64(newVolumeGB * float64(models.GB))
	_, err := DB.Exec("UPDATE users SET data_limit = ? WHERE username = ?", dataLimit, username)
	return err
}

const userColumns = "id, username, password, expiry_date, data_limit, data_used, IFNULL(last_seen, 0), IFNULL(sub_token, ''), IFNULL(udpgw_port, 7301), IFNULL(comment, '')"

func scanUser(s interface{ Scan(...interface{}) error }, u *User) error {
	return s.Scan(&u.ID, &u.Username, &u.Password, &u.ExpiryDate, &u.DataLimit, &u.DataUsed, &u.LastSeen, &u.SubToken, &u.UdpgwPort, &u.Comment)
}

func GetUsers() ([]User, error) {
	rows, err := DB.Query("SELECT " + userColumns + " FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		if err := scanUser(rows, &u); err == nil {
			users = append(users, u)
		}
	}
	return users, nil
}

func GetUserBySubToken(token string) (User, error) {
	var u User
	err := scanUser(DB.QueryRow("SELECT "+userColumns+" FROM users WHERE sub_token = ?", token), &u)
	return u, err
}

func IncrementUserDataUsed(username string, bytes int64, nodeIP string) error {
	if nodeIP == "" {
		nodeIP = "Main-Server"
	}
	if bytes > 0 {
		if _, err := DB.Exec("UPDATE users SET data_used = data_used + ? WHERE username = ?", bytes, username); err != nil {
			return err
		}
	}

	now := time.Now().In(models.IranTime)
	logDate := now.Format("2006-01-02")
	logHour := now.Hour()

	if bytes > 0 {
		_, _ = DB.Exec("INSERT INTO traffic_logs (username, log_date, log_hour, node_ip, bytes_used) VALUES (?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE bytes_used = bytes_used + ?", username, logDate, logHour, nodeIP, bytes, bytes)
		_, _ = DB.Exec("INSERT INTO node_traffic_logs (ip, log_date, log_hour, bytes_used) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE bytes_used = bytes_used + ?", nodeIP, logDate, logHour, bytes, bytes)
	}

	_, _ = DB.Exec("INSERT INTO nodes (ip, total_traffic, last_seen) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE total_traffic = total_traffic + ?, last_seen = ?", nodeIP, bytes, now.Unix(), bytes, now.Unix())
	return nil
}

func ResetUserDataUsed(username string) error {
	_, err := DB.Exec("UPDATE users SET data_used = 0 WHERE username = ?", username)
	return err
}

func ResetUserExpiry(username string, daysFromToday int) error {
	expiryDate := time.Now().AddDate(0, 0, daysFromToday)
	_, err := DB.Exec("UPDATE users SET expiry_date = ? WHERE username = ?", expiryDate, username)
	return err
}

func UpdateLastSeen(username string, timestamp int64) error {
	_, err := DB.Exec("UPDATE users SET last_seen = ? WHERE username = ?", timestamp, username)
	return err
}

// ---------------------------------------------------------------------------
// Traffic statistics
// ---------------------------------------------------------------------------

func GetUserTrafficStats(username string) TrafficStats {
	var stats TrafficStats
	now := time.Now().In(models.IranTime)
	todayStr := now.Format("2006-01-02")
	yesterdayStr := now.AddDate(0, 0, -1).Format("2006-01-02")
	weekAgoStr := now.AddDate(0, 0, -7).Format("2006-01-02")
	monthAgoStr := now.AddDate(0, -1, 0).Format("2006-01-02")
	month3AgoStr := now.AddDate(0, -3, 0).Format("2006-01-02")

	getSum := func(condition string, args ...interface{}) float64 {
		query := fmt.Sprintf("SELECT IFNULL(SUM(bytes_used), 0) FROM traffic_logs WHERE username = ? AND %s", condition)
		var bytes int64
		fullArgs := append([]interface{}{username}, args...)
		_ = DB.QueryRow(query, fullArgs...).Scan(&bytes)
		return float64(bytes) / float64(models.GB)
	}

	getHoursSum := func(hours int) float64 {
		tStr := now.Add(-time.Duration(hours) * time.Hour).Format("2006-01-02 15:04:05")
		var bytes int64
		_ = DB.QueryRow("SELECT IFNULL(SUM(bytes_used), 0) FROM traffic_logs WHERE username = ? AND TIMESTAMP(log_date, MAKETIME(log_hour, 0, 0)) >= ?", username, tStr).Scan(&bytes)
		return float64(bytes) / float64(models.GB)
	}

	stats.H1 = getHoursSum(1)
	stats.H2 = getHoursSum(2)
	stats.H6 = getHoursSum(6)
	stats.H12 = getHoursSum(12)
	stats.Today = getSum("log_date = ?", todayStr)
	stats.Yesterday = getSum("log_date = ?", yesterdayStr)
	stats.ThisWeek = getSum("log_date >= ?", weekAgoStr)
	stats.ThisMonth = getSum("log_date >= ?", monthAgoStr)
	stats.M3 = getSum("log_date >= ?", month3AgoStr)
	return stats
}

// GetNodeTrafficStats aggregates traffic processed by a specific node.
// (Previously this returned all zeros, so the node analytics screen was blank.)
func GetNodeTrafficStats(ip string) TrafficStats {
	var stats TrafficStats
	now := time.Now().In(models.IranTime)
	todayStr := now.Format("2006-01-02")
	yesterdayStr := now.AddDate(0, 0, -1).Format("2006-01-02")
	weekAgoStr := now.AddDate(0, 0, -7).Format("2006-01-02")
	monthAgoStr := now.AddDate(0, -1, 0).Format("2006-01-02")
	month3AgoStr := now.AddDate(0, -3, 0).Format("2006-01-02")

	getSum := func(condition string, args ...interface{}) float64 {
		query := fmt.Sprintf("SELECT IFNULL(SUM(bytes_used), 0) FROM node_traffic_logs WHERE ip = ? AND %s", condition)
		var bytes int64
		fullArgs := append([]interface{}{ip}, args...)
		_ = DB.QueryRow(query, fullArgs...).Scan(&bytes)
		return float64(bytes) / float64(models.GB)
	}

	getHoursSum := func(hours int) float64 {
		tStr := now.Add(-time.Duration(hours) * time.Hour).Format("2006-01-02 15:04:05")
		var bytes int64
		_ = DB.QueryRow("SELECT IFNULL(SUM(bytes_used), 0) FROM node_traffic_logs WHERE ip = ? AND TIMESTAMP(log_date, MAKETIME(log_hour, 0, 0)) >= ?", ip, tStr).Scan(&bytes)
		return float64(bytes) / float64(models.GB)
	}

	stats.H1 = getHoursSum(1)
	stats.H2 = getHoursSum(2)
	stats.H6 = getHoursSum(6)
	stats.H12 = getHoursSum(12)
	stats.Today = getSum("log_date = ?", todayStr)
	stats.Yesterday = getSum("log_date = ?", yesterdayStr)
	stats.ThisWeek = getSum("log_date >= ?", weekAgoStr)
	stats.ThisMonth = getSum("log_date >= ?", monthAgoStr)
	stats.M3 = getSum("log_date >= ?", month3AgoStr)
	return stats
}

// ---------------------------------------------------------------------------
// Nodes
// ---------------------------------------------------------------------------

func UpdateNodeLastSeen(ip string, timestamp int64) error {
	_, err := DB.Exec("INSERT INTO nodes (ip, last_seen) VALUES (?, ?) ON DUPLICATE KEY UPDATE last_seen = ?", ip, timestamp, timestamp)
	return err
}

func UpdateNodeSettings(ip, domain, remark string) error {
	query := `INSERT INTO nodes (ip, name, domain, custom_remark)
	          VALUES (?, ?, ?, ?)
	          ON DUPLICATE KEY UPDATE domain = ?, custom_remark = ?, name = ?`
	_, err := DB.Exec(query, ip, remark, domain, remark, domain, remark, remark)
	return err
}

func GetNodes() ([]Node, error) {
	rows, err := DB.Query("SELECT ip, last_seen, total_traffic, IFNULL(name, ''), IFNULL(domain, ''), IFNULL(custom_remark, '') FROM nodes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.IP, &n.LastSeen, &n.TotalTraffic, &n.Name, &n.Domain, &n.CustomRemark); err == nil {
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

// DeleteNode removes a node from the panel's node list ("nodes" table) only.
// It intentionally does NOT touch node_traffic_logs or traffic_logs: neither
// table has a foreign key back to nodes.ip, so deleting the node's row here
// can never cascade and wipe out historical traffic data. Per-user usage
// totals (users.data_used) and panel-wide stats are computed independently
// of whether the node still exists, so they are unaffected as well.
//
// Note: if the same IP later calls the node API again (heartbeat/usage
// report), it will simply be re-inserted with a fresh total_traffic counter
// starting from 0 - the detailed history in node_traffic_logs is untouched
// and keeps accumulating under that IP, so GetNodeTrafficStats/GetNodeChartData
// still report the full historical figures even across a delete+reappear.
func DeleteNode(ip string) error {
	_, err := DB.Exec("DELETE FROM nodes WHERE ip = ?", ip)
	return err
}

func GetNodeChartData(ip string) []float64 {
	now := time.Now().In(models.IranTime)

	fetchHourly := func(hours int) float64 {
		tStr := now.Add(time.Duration(-hours) * time.Hour).Format("2006-01-02 15:00:00")
		var b int64
		_ = DB.QueryRow("SELECT IFNULL(SUM(bytes_used), 0) FROM node_traffic_logs WHERE ip = ? AND TIMESTAMP(log_date, MAKETIME(log_hour, 0, 0)) >= ?", ip, tStr).Scan(&b)
		return float64(b) / float64(models.GB)
	}
	fetchDaily := func(days int) float64 {
		tStr := now.AddDate(0, 0, -days).Format("2006-01-02")
		var b int64
		_ = DB.QueryRow("SELECT IFNULL(SUM(bytes_used), 0) FROM node_traffic_logs WHERE ip = ? AND log_date >= ?", ip, tStr).Scan(&b)
		return float64(b) / float64(models.GB)
	}
	fetchExactDay := func(daysAgo int) float64 {
		tStr := now.AddDate(0, 0, -daysAgo).Format("2006-01-02")
		var b int64
		_ = DB.QueryRow("SELECT IFNULL(SUM(bytes_used), 0) FROM node_traffic_logs WHERE ip = ? AND log_date = ?", ip, tStr).Scan(&b)
		return float64(b) / float64(models.GB)
	}

	return []float64{
		fetchHourly(1),
		fetchHourly(2),
		fetchHourly(6),
		fetchHourly(12),
		fetchHourly(24),
		fetchExactDay(0),
		fetchExactDay(1),
		fetchDaily(3),
		fetchDaily(7),
		fetchDaily(30),
		fetchDaily(90),
	}
}
// ---------------------------------------------------------------------------
// Exported wrappers so the api package can build mysqldump / mysql commands
// with the same credentials the Go connection uses, without duplicating the
// DSN-parsing logic and without putting the password on the command line.
// ---------------------------------------------------------------------------

func DBCredentials() (user, pass string) { return dbCredentials() }

func WriteMySQLDefaultsFile(user, pass string) (string, func(), error) {
	return writeMySQLDefaultsFile(user, pass)
}
