// Command ipan 启动 iPan 服务：联通云盘 Web 管理端（Go + Vue 版）。
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ipan/internal/config"
	"ipan/internal/httpapi"
	"ipan/internal/store"
)

func main() {
	var (
		listen    = flag.String("listen", "", "监听地址，覆盖 IPAN_LISTEN")
		dataDir   = flag.String("data", "", "数据目录，覆盖 IPAN_DATA_DIR")
		staticDir = flag.String("static", "./web/dist", "前端构建产物目录")
	)
	flag.Parse()

	cfg := config.Load()
	if *listen != "" {
		cfg.Listen = *listen
	}
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	}

	st, err := store.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("打开数据目录失败: %v", err)
	}
	defer st.Close()
	log.Printf("数据库: %s", st.Path)

	if *staticDir != "" {
		if info, serr := os.Stat(*staticDir); serr != nil || !info.IsDir() {
			log.Printf("[WARN] 前端目录不存在: %s（仅提供 API）", *staticDir)
			*staticDir = ""
		}
	}

	srv := httpapi.NewServer(cfg, st, *staticDir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopMaintenance := srv.StartTokenMaintenance(ctx)
	defer stopMaintenance()

	// 定期清理过期会话与分享
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = st.PurgeExpired()
			case <-ctx.Done():
				return
			}
		}
	}()

	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		// 上传/下载可能耗时很久，这里不设总超时，靠客户端断开与 IdleTimeout 回收
		IdleTimeout: 120 * time.Second,
	}

	go func() {
		log.Printf("iPan %s 已启动: http://%s", httpapi.Version, displayAddr(cfg.Listen))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("正在关闭服务…")
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("关闭服务时出错: %v", err)
	}
}

func displayAddr(addr string) string {
	if len(addr) > 0 && addr[0] == ':' {
		return "localhost" + addr
	}
	return addr
}
