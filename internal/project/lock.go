package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// LockInfo represents the lock file content
type LockInfo struct {
	PID       int       `json:"pid"`
	Timestamp time.Time `json:"timestamp"`
	Hostname  string    `json:"hostname"`
}

// LockFile path relative to project root
const LockFile = ".teeter/lock"

// AcquireLock attempts to acquire a lock for the project.
// Returns an unlock function and error.
// Lock is stale if PID doesn't exist or lock is older than 1 hour.
func AcquireLock(workDir string) (func() error, error) {
	lockPath := filepath.Join(workDir, LockFile)

	// Ensure .teeter directory exists
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}

	// Check existing lock
	if info, err := readLock(lockPath); err == nil {
		if !isStale(info) {
			return nil, fmt.Errorf("project locked by PID %d (started %s)", info.PID, info.Timestamp.Format(time.RFC3339))
		}
		// Stale lock - remove it
		_ = os.Remove(lockPath)
	}

	// Write new lock
	info := LockInfo{
		PID:       os.Getpid(),
		Timestamp: time.Now(),
		Hostname:  getHostname(),
	}
	if err := writeLock(lockPath, info); err != nil {
		return nil, fmt.Errorf("write lock: %w", err)
	}

	// Return unlock function
	return func() error {
		return os.Remove(lockPath)
	}, nil
}

// IsLocked checks if the project is currently locked (non-stale)
func IsLocked(workDir string) bool {
	lockPath := filepath.Join(workDir, LockFile)
	info, err := readLock(lockPath)
	if err != nil {
		return false
	}
	return !isStale(info)
}

// GetLockInfo returns the lock info if locked, nil otherwise
func GetLockInfo(workDir string) *LockInfo {
	lockPath := filepath.Join(workDir, LockFile)
	info, err := readLock(lockPath)
	if err != nil {
		return nil
	}
	if isStale(info) {
		return nil
	}
	return info
}

func readLock(path string) (*LockInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var info LockInfo
	if err := parseLock(string(data), &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func writeLock(path string, info LockInfo) error {
	data := formatLock(info)
	return os.WriteFile(path, []byte(data), 0644)
}

func formatLock(info LockInfo) string {
	return fmt.Sprintf("pid:%d\ntimestamp:%s\nhostname:%s\n",
		info.PID, info.Timestamp.Format(time.RFC3339), info.Hostname)
}

func parseLock(data string, info *LockInfo) error {
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		if line[0] == '#' {
			continue
		}
		parts := splitN(line, ':', 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		val := parts[1]
		switch key {
		case "pid":
			pid, _ := strconv.Atoi(val)
			info.PID = pid
		case "timestamp":
			t, _ := time.Parse(time.RFC3339, val)
			info.Timestamp = t
		case "hostname":
			info.Hostname = val
		}
	}
	return nil
}

func splitLines(s string) []string {
	var lines []string
	var current string
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func splitN(s string, sep rune, n int) []string {
	var parts []string
	var current string
	for _, r := range s {
		if r == sep && len(parts) < n-1 {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	parts = append(parts, current)
	return parts
}

func isStale(info *LockInfo) bool {
	// Check if process is still alive
	process, err := os.FindProcess(info.PID)
	if err != nil {
		return true
	}
	// On Unix, FindProcess always succeeds; send signal 0 to check
	err = process.Signal(os.Signal(nil))
	if err != nil {
		return true
	}
	// Check age > 1 hour
	if time.Since(info.Timestamp) > time.Hour {
		return true
	}
	return false
}

func getHostname() string {
	hostname, _ := os.Hostname()
	return hostname
}
