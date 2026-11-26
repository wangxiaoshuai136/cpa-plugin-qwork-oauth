//go:build windows

package cpa

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func exists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func browserExecutable(pref string) string {
	local := os.Getenv("LOCALAPPDATA")
	pf := os.Getenv("ProgramFiles")
	pfx := os.Getenv("ProgramFiles(x86)")
	edge := []string{
		filepath.Join(local, `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(pfx, `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(pf, `Microsoft\Edge\Application\msedge.exe`),
	}
	chrome := []string{
		filepath.Join(local, `Google\Chrome\Application\chrome.exe`),
		filepath.Join(pf, `Google\Chrome\Application\chrome.exe`),
		filepath.Join(pfx, `Google\Chrome\Application\chrome.exe`),
	}
	brave := []string{
		filepath.Join(local, `BraveSoftware\Brave-Browser\Application\brave.exe`),
		filepath.Join(pf, `BraveSoftware\Brave-Browser\Application\brave.exe`),
	}
	firefox := []string{
		filepath.Join(pf, `Mozilla Firefox\firefox.exe`),
		filepath.Join(pfx, `Mozilla Firefox\firefox.exe`),
	}
	pick := func(list []string) string {
		for _, p := range list {
			if exists(p) {
				return p
			}
		}
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(pref)) {
	case "edge":
		return pick(edge)
	case "chrome":
		return pick(chrome)
	case "brave":
		return pick(brave)
	case "firefox":
		return pick(firefox)
	default:
		if p := pick(edge); p != "" {
			return p
		}
		return pick(chrome)
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// openIsolatedBrowser launches the selected browser with a throwaway profile so
// the login does not touch the user's existing cookies.
func openIsolatedBrowser(pref, url string) bool {
	exe := browserExecutable(pref)
	if exe == "" {
		return false
	}
	dir := filepath.Join(os.TempDir(), "qwork-oauth-"+randomHex(6))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	var args []string
	if strings.Contains(strings.ToLower(filepath.Base(exe)), "firefox") {
		args = []string{"-new-instance", "-profile", dir, url}
	} else {
		args = []string{"--user-data-dir=" + dir, "--no-first-run", "--no-default-browser-check", "--new-window", url}
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := cmd.Start(); err != nil {
		return false
	}
	go func() { _ = cmd.Wait() }()
	return true
}
