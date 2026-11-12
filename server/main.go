package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	dbPath := sqlitePath()
	stats, err := OpenStats(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer stats.Close()

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	created, accessKey, otpauth, err := stats.EnsureAdmin(bootCtx)
	bootCancel()
	if err != nil {
		log.Fatal(err)
	}
	if created {
		file := filepath.Join(filepath.Dir(dbPath), "bootstrap.txt")
		body := fmt.Sprintf("Access Key:\n%s\n\nTOTP:\n%s\n\n把密钥加入验证器后删除本文件。Access Key 不会再次显示。\n", accessKey, otpauth)
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			log.Fatal(err)
		}
		log.Printf("已生成 Access Key 和 TOTP，凭证只写在 %s", file)
	}

	app := newApp(stats)
	interval := probeInterval()

	r := gin.Default()
	if err := r.SetTrustedProxies(nil); err != nil {
		log.Fatal(err)
	}
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})
	app.mount(r)
	mountWeb(r)

	go sampleLoop(app, interval)

	addr := os.Getenv("DBA_ADDR")
	if addr == "" {
		addr = "0.0.0.0:8080"
	}
	log.Printf("capacity sampler every %s", interval)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

func sampleLoop(app *App, interval time.Duration) {
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if _, err := app.probeAll(ctx); err != nil {
			log.Printf("capacity sample: %v", err)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		run()
	}
}

func probeInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("DBA_PROBE_INTERVAL"))
	if raw == "" {
		return time.Hour
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < time.Minute {
		log.Fatalf("DBA_PROBE_INTERVAL 至少为 1m，当前是 %q", raw)
	}
	return d
}

func sqlitePath() string {
	if p := strings.TrimSpace(os.Getenv("DBA_SQLITE")); p != "" {
		return p
	}
	return filepath.Join("data", "stats.db")
}

func mountWeb(r *gin.Engine) {
	dist := strings.TrimSpace(os.Getenv("DBA_WEB_DIST"))
	if dist == "" {
		for _, candidate := range []string{filepath.Join("..", "web", "dist"), filepath.Join("web", "dist")} {
			if _, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil {
				dist = candidate
				break
			}
		}
	}
	if dist == "" {
		log.Printf("未找到 web/dist，仅提供 API")
		r.NoRoute(func(c *gin.Context) {
			fail(c, http.StatusNotFound, "not found")
		})
		return
	}
	abs, err := filepath.Abs(dist)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("serving web from %s", abs)
	fs := http.Dir(abs)
	fileServer := http.FileServer(fs)
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			fail(c, http.StatusNotFound, "not found")
			return
		}
		rel := strings.TrimPrefix(path.Clean("/"+c.Request.URL.Path), "/")
		if rel == "" || rel == "." {
			c.File(filepath.Join(abs, "index.html"))
			return
		}
		f, err := fs.Open(rel)
		if err != nil {
			c.File(filepath.Join(abs, "index.html"))
			return
		}
		info, statErr := f.Stat()
		f.Close()
		if statErr != nil || info.IsDir() {
			c.File(filepath.Join(abs, "index.html"))
			return
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
