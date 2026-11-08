// Package config loads NaniPlay runtime configuration from the environment.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds the runtime settings for the server.
type Config struct {
	Host      string // APP_HOST, default 127.0.0.1
	Port      int    // APP_PORT, default 8080
	DataDir   string // NANIPLAY_DATA, default ./data
	OtakuBase string // OTAKU_BASE, default https://ww2.otakudesu.biz
	HiBase    string // HIANIME_BASE, default https://hianime.at
	IdlixBase string // IDLIX_BASE, default https://z2.idlixku.com
	NoOpen    bool   // NANIPLAY_NO_OPEN=1 disables auto-opening the browser
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// Load reads configuration from environment variables.
func Load() Config {
	port, err := strconv.Atoi(getenv("APP_PORT", "8080"))
	if err != nil || port <= 0 || port > 65535 {
		port = 8080
	}
	dataDir := getenv("NANIPLAY_DATA", "")
	if dataDir == "" {
		if wd, err := os.Getwd(); err == nil {
			dataDir = filepath.Join(wd, "data")
		} else {
			dataDir = "data"
		}
	}
	return Config{
		Host:      getenv("APP_HOST", "127.0.0.1"),
		Port:      port,
		DataDir:   dataDir,
		OtakuBase: strings.TrimRight(getenv("OTAKU_BASE", "https://ww2.otakudesu.biz"), "/"),
		HiBase:    strings.TrimRight(getenv("HIANIME_BASE", "https://hianime.at"), "/"),
		IdlixBase: strings.TrimRight(getenv("IDLIX_BASE", "https://z2.idlixku.com"), "/"),
		NoOpen:    os.Getenv("NANIPLAY_NO_OPEN") == "1",
	}
}
