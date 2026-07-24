package sshvpn

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/asd1asd00000/svm-panel/database"
	"github.com/asd1asd00000/svm-panel/models"
)

type SyncUser struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	DataLimit  int64  `json:"data_limit"`
	DataUsed   int64  `json:"data_used"`
	ExpiryUnix int64  `json:"expiry_unix"`
}

type NodeSession struct {
	Limit int64
	Used  int64
	Conns []ssh.Conn
}

var (
	activeUsers       = make(map[string]int)
	activeUsersMutex  sync.Mutex
	localUsers        = make(map[string]SyncUser)
	localUsersMutex   sync.RWMutex
	nodeSessions      = make(map[string]*NodeSession)
	nodeSessionsMutex sync.Mutex
	pendingUsage      = make(map[string]int64)
	pendingUsageMutex sync.Mutex
	nodeOnlineCache   = make(map[string]int64)
	nodeOnlineMutex   sync.Mutex
)

func UpdateNodeOnlineStatus(users []string) {
	nodeOnlineMutex.Lock()
	now := time.Now().Unix()
	for _, u := range users {
		nodeOnlineCache[u] = now
	}
	nodeOnlineMutex.Unlock()
}

// GetOnlineUsersList returns users online either locally (main server) or on a
// connected node (reported within the last 30 seconds).
func GetOnlineUsersList() []string {
	var list []string
	now := time.Now().Unix()
	onlineMap := make(map[string]bool)

	activeUsersMutex.Lock()
	for u, count := range activeUsers {
		if count > 0 {
			onlineMap[u] = true
		}
	}
	activeUsersMutex.Unlock()

	nodeOnlineMutex.Lock()
	for u, lastSeen := range nodeOnlineCache {
		if now-lastSeen < 30 {
			onlineMap[u] = true
		}
	}
	nodeOnlineMutex.Unlock()

	for u := range onlineMap {
		list = append(list, u)
	}
	return list
}

func IsUserOnline(username string) bool {
	activeUsersMutex.Lock()
	localOnline := activeUsers[username] > 0
	activeUsersMutex.Unlock()
	if localOnline {
		return true
	}

	nodeOnlineMutex.Lock()
	lastSeen := nodeOnlineCache[username]
	nodeOnlineMutex.Unlock()
	return time.Now().Unix()-lastSeen < 30
}

type chanWriter struct {
	w        io.Writer
	username string
}

func (cw chanWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	if n > 0 {
		nodeSessionsMutex.Lock()
		if sess, exists := nodeSessions[cw.username]; exists {
			sess.Used += int64(n)
		}
		nodeSessionsMutex.Unlock()

		pendingUsageMutex.Lock()
		pendingUsage[cw.username] += int64(n)
		pendingUsageMutex.Unlock()
	}
	return n, err
}

// hostKeyPath returns where the persistent SSH host key is stored.
func hostKeyPath() string {
	if p := os.Getenv("SVM_SSH_HOST_KEY"); p != "" {
		return p
	}
	return "/root/svm-panel/ssh_host_key"
}

// loadOrCreateHostKey loads a persistent RSA host key from disk, generating and
// saving one on first run. This stops clients from seeing a different host key
// (and a MITM warning) on every restart.
func loadOrCreateHostKey() (ssh.Signer, error) {
	path := hostKeyPath()
	if data, err := os.ReadFile(path); err == nil {
		if signer, err := ssh.ParsePrivateKey(data); err == nil {
			return signer, nil
		}
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate host key: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	if err := os.WriteFile(path, pemBytes, 0600); err != nil {
		// Non-fatal: fall back to the in-memory key for this run.
		log.Printf("warning: could not persist SSH host key to %s: %v", path, err)
	}
	return ssh.ParsePrivateKey(pemBytes)
}

// ---------------------------------------------------------------------------
// Node -> Main HTTP helpers.
// The cluster token is sent in the X-Auth-Token header instead of the URL
// query string, so it does not end up in nginx access logs or proxy logs on
// the path between the node and the main server.
// ---------------------------------------------------------------------------

func nodeRequest(client *http.Client, method, url, token string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("X-Auth-Token", token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return client.Do(req)
}

func StartSSHServer(listenAddr string, isNode bool, mainURL, token string) {
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if !isNode {
				var dbPassword string
				var expiryDate time.Time
				var dataLimit, dataUsed int64
				query := "SELECT password, expiry_date, data_limit, data_used FROM users WHERE username = ?"
				err := database.DB.QueryRow(query, c.User()).Scan(&dbPassword, &expiryDate, &dataLimit, &dataUsed)
				if err != nil {
					return nil, fmt.Errorf("auth failed")
				}
				if subtle.ConstantTimeCompare(password, []byte(dbPassword)) != 1 {
					return nil, fmt.Errorf("auth failed")
				}
				if time.Now().After(expiryDate) || models.OverLimit(dataLimit, dataUsed) {
					return nil, fmt.Errorf("auth failed")
				}
				return nil, nil
			}

			localUsersMutex.RLock()
			user, exists := localUsers[c.User()]
			localUsersMutex.RUnlock()

			if !exists || subtle.ConstantTimeCompare(password, []byte(user.Password)) != 1 {
				return nil, fmt.Errorf("invalid credentials")
			}
			if time.Now().Unix() > user.ExpiryUnix {
				return nil, fmt.Errorf("account expired")
			}
			if models.OverLimit(user.DataLimit, user.DataUsed) {
				return nil, fmt.Errorf("data limit reached")
			}
			return nil, nil
		},
	}

	signer, err := loadOrCreateHostKey()
	if err != nil {
		log.Fatalf("SSH host key error: %v", err)
	}
	config.AddHostKey(signer)

	if isNode {
		// Periodically pull the user list from the main server.
		go func() {
			httpClient := &http.Client{Timeout: 5 * time.Second}
			for {
				reqURL := fmt.Sprintf("%s/api/users", mainURL)
				resp, err := nodeRequest(httpClient, http.MethodGet, reqURL, token, nil)
				if err == nil {
					var list []SyncUser
					if json.NewDecoder(resp.Body).Decode(&list) == nil {
						localUsersMutex.Lock()
						for _, u := range list {
							localUsers[u.Username] = u
						}
						localUsersMutex.Unlock()
					}
					resp.Body.Close()
				}
				time.Sleep(15 * time.Second)
			}
		}()

		// Report online users + flush pending usage to the main server.
		go func() {
			usageClient := &http.Client{Timeout: 5 * time.Second}
			for {
				time.Sleep(4 * time.Second)

				var onlineList []string
				activeUsersMutex.Lock()
				for u, count := range activeUsers {
					if count > 0 {
						onlineList = append(onlineList, u)
					}
				}
				activeUsersMutex.Unlock()

				jsonBytesOnline, _ := json.Marshal(onlineList)
				reqURLOnline := fmt.Sprintf("%s/api/online", mainURL)
				respOnline, errOnline := nodeRequest(usageClient, http.MethodPost, reqURLOnline, token, bytes.NewBuffer(jsonBytesOnline))
				if errOnline == nil {
					respOnline.Body.Close()
				}

				pendingUsageMutex.Lock()
				snapshot := make(map[string]int64)
				for u, b := range pendingUsage {
					if b > 0 {
						snapshot[u] = b
						pendingUsage[u] = 0
					}
				}
				pendingUsageMutex.Unlock()

				for user, bytesAdded := range snapshot {
					payload := map[string]interface{}{"username": user, "bytes_added": bytesAdded}
					jsonBytesUsage, _ := json.Marshal(payload)
					reqURLUsage := fmt.Sprintf("%s/api/usage", mainURL)
					respUsage, errUsage := nodeRequest(usageClient, http.MethodPost, reqURLUsage, token, bytes.NewBuffer(jsonBytesUsage))
					if errUsage == nil {
						respUsage.Body.Close()
					} else {
						// Re-queue the usage we failed to report.
						pendingUsageMutex.Lock()
						pendingUsage[user] += bytesAdded
						pendingUsageMutex.Unlock()
					}
				}

				nodeSessionsMutex.Lock()
				for username, sess := range nodeSessions {
					localUsersMutex.RLock()
					lu, exists := localUsers[username]
					localUsersMutex.RUnlock()
					if exists && (models.OverLimit(lu.DataLimit, lu.DataUsed) || models.OverLimit(lu.DataLimit, sess.Used)) {
						for _, conn := range sess.Conns {
							conn.Close()
						}
					}
				}
				nodeSessionsMutex.Unlock()
			}
		}()
	} else {
		// Main server: flush pending usage to the DB and enforce limits.
		go func() {
			for {
				time.Sleep(3 * time.Second)
				now := time.Now().Unix()
				_ = database.UpdateNodeLastSeen("Main-Server", now)

				pendingUsageMutex.Lock()
				snapshot := make(map[string]int64)
				for u, b := range pendingUsage {
					if b > 0 {
						snapshot[u] = b
						pendingUsage[u] = 0
					}
				}
				pendingUsageMutex.Unlock()

				for user, bytesAdded := range snapshot {
					_ = database.IncrementUserDataUsed(user, bytesAdded, "Main-Server")
				}

				nodeSessionsMutex.Lock()
				for username, sess := range nodeSessions {
					_ = database.UpdateLastSeen(username, now)
					var dataLimit, dataUsed int64
					err := database.DB.QueryRow("SELECT data_limit, data_used FROM users WHERE username = ?", username).Scan(&dataLimit, &dataUsed)
					if err == nil && (models.OverLimit(dataLimit, dataUsed) || models.OverLimit(dataLimit, sess.Used)) {
						for _, conn := range sess.Conns {
							conn.Close()
						}
					}
				}
				nodeSessionsMutex.Unlock()
			}
		}()
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go handleConnection(conn, config, isNode)
	}
}

func handleConnection(nConn net.Conn, config *ssh.ServerConfig, isNode bool) {
	sConn, chans, reqs, err := ssh.NewServerConn(nConn, config)
	if err != nil {
		return
	}
	username := sConn.User()

	activeUsersMutex.Lock()
	activeUsers[username]++
	activeUsersMutex.Unlock()

	nodeSessionsMutex.Lock()
	sess, exists := nodeSessions[username]
	if !exists {
		var dataLimit, dataUsed int64
		if !isNode {
			_ = database.DB.QueryRow("SELECT data_limit, data_used FROM users WHERE username = ?", username).Scan(&dataLimit, &dataUsed)
		} else {
			localUsersMutex.RLock()
			lu := localUsers[username]
			localUsersMutex.RUnlock()
			dataLimit = lu.DataLimit
			dataUsed = lu.DataUsed
		}
		sess = &NodeSession{Limit: dataLimit, Used: dataUsed, Conns: []ssh.Conn{}}
		nodeSessions[username] = sess
	}
	sess.Conns = append(sess.Conns, sConn)
	nodeSessionsMutex.Unlock()

	defer func() {
		sConn.Close()
		nodeSessionsMutex.Lock()
		if s := nodeSessions[username]; s != nil {
			for i, c := range s.Conns {
				if c == sConn {
					s.Conns = append(s.Conns[:i], s.Conns[i+1:]...)
					break
				}
			}
			if len(s.Conns) == 0 {
				delete(nodeSessions, username)
			}
		}
		nodeSessionsMutex.Unlock()

		activeUsersMutex.Lock()
		if activeUsers[username] > 0 {
			activeUsers[username]--
		}
		if activeUsers[username] == 0 {
			delete(activeUsers, username)
		}
		activeUsersMutex.Unlock()
	}()

	go ssh.DiscardRequests(reqs)
	for newChannel := range chans {
		if newChannel.ChannelType() != "direct-tcpip" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		go handleProxyChannel(username, newChannel)
	}
}

func handleProxyChannel(username string, newChannel ssh.NewChannel) {
	type channelOpenDirectMsg struct {
		Raddr string
		Rport uint32
	}
	var msg channelOpenDirectMsg
	_ = ssh.Unmarshal(newChannel.ExtraData(), &msg)

	targetConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", msg.Raddr, msg.Rport))
	if err != nil {
		newChannel.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	defer targetConn.Close()

	connection, requests, err := newChannel.Accept()
	if err != nil {
		return
	}
	defer connection.Close()
	go ssh.DiscardRequests(requests)

	go func() {
		cw := chanWriter{w: connection, username: username}
		_, _ = io.Copy(cw, targetConn)
		connection.CloseWrite()
	}()

	cw2 := chanWriter{w: targetConn, username: username}
	_, _ = io.Copy(cw2, connection)
}
