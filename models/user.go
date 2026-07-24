package models

import "time"

// IranTime is the fixed IRST (+03:30) timezone used across the whole panel.
// Defined once here so it is not re-created in every file.
var IranTime = time.FixedZone("IRST", 12600)

// Byte size units (binary).
const (
	KB int64 = 1024
	MB       = 1024 * KB
	GB       = 1024 * MB
)

// User is the canonical account model shared by all packages.
type User struct {
	ID         int
	Username   string
	Password   string
	ExpiryDate time.Time
	DataLimit  int64 // in bytes, 0 = unlimited
	DataUsed   int64 // in bytes
	LastSeen   int64 // unix seconds
	SubToken   string
	UdpgwPort  int
	Comment    string
}

// IsExpired reports whether the account is past its expiry date.
func (u User) IsExpired() bool { return time.Now().After(u.ExpiryDate) }

// IsOverLimit reports whether the account exhausted its data quota.
// A DataLimit of 0 means unlimited, so it can never be over limit.
func (u User) IsOverLimit() bool { return u.DataLimit > 0 && u.DataUsed >= u.DataLimit }

// IsActive reports whether the account may currently connect.
func (u User) IsActive() bool { return !u.IsExpired() && !u.IsOverLimit() }

// OverLimit is a helper for raw (limit, used) pairs where a full User is not
// available (e.g. inside SSH auth callbacks). 0 limit means unlimited.
func OverLimit(limit, used int64) bool { return limit > 0 && used >= limit }

// TrafficStats holds aggregated traffic figures for various time windows (GB).
type TrafficStats struct {
	H1        float64
	H2        float64
	H6        float64
	H12       float64
	Today     float64
	Yesterday float64
	ThisWeek  float64
	ThisMonth float64
	M3        float64
}

// Node is a connected server (main or edge node).
type Node struct {
	IP           string
	LastSeen     int64
	TotalTraffic int64
	Name         string
	Domain       string
	CustomRemark string
}
