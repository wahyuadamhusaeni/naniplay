// NaniPlay — anime streaming aggregator with Indonesian subtitles.
//
// One binary serves the web UI (self-hosted) and, on Windows, doubles as
// the desktop app: it starts the local server and opens the browser.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/wahyuadamhusaeni/naniplay/internal/config"
	"github.com/wahyuadamhusaeni/naniplay/internal/store"
	"github.com/wahyuadamhusaeni/naniplay/internal/web"
)

var version = "1.9.1"

func main() {
	log.SetPrefix("[naniplay] ")
	cfg := config.Load()

	db, err := store.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	web.Version = version
	srv, err := web.New(db, cfg.OtakuBase, cfg.HiBase, cfg.IdlixBase, cfg.KitaBase)
	if err != nil {
		log.Fatalf("web: %v", err)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	url := fmt.Sprintf("http://%s", addr)
	if cfg.Host == "0.0.0.0" {
		url = fmt.Sprintf("http://127.0.0.1:%d", cfg.Port)
	}
	log.Printf("NaniPlay %s — Nani?! Play. %s", version, url)

	if !cfg.NoOpen && cfg.Host != "0.0.0.0" {
		go func() {
			time.Sleep(600 * time.Millisecond)
			openBrowser(url)
		}()
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("buka browser manual ya: %s (%v)", url, err)
	}
}

var _ = os.Getenv // keep os import if flags are added later
