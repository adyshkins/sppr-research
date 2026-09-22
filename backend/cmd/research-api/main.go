package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"sppr-prototype/research/internal/httpapi"
	"sppr-prototype/research/internal/r08"
	"sppr-prototype/research/internal/research"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18081", "loopback address only")
	data := flag.String("data", "./data", "local private directory for imported bundles")
	origin := flag.String("origins", "http://localhost:5173,http://127.0.0.1:5173", "exact browser origins")
	binary := flag.String("engine", "", "path to preserved recoverycheck binary; empty disables R08")
	plan := flag.String("plan", "", "path to original DEV-R08 plan")
	static := flag.String("static", "", "optional built frontend directory")
	flag.Parse()
	host, _, err := net.SplitHostPort(*addr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		slog.Error("addr must be a numeric loopback IP and port")
		os.Exit(2)
	}
	store, err := research.NewStore(*data)
	if err != nil {
		slog.Error("cannot open storage", "error", err)
		os.Exit(1)
	}
	var engine *r08.Manager
	if *binary != "" {
		engine, err = r08.New(filepath.Join(*data, "r08"), *binary, *plan)
		if err != nil {
			slog.Error("R08 initialization failed", "error", err)
			os.Exit(1)
		}
		defer engine.Close()
	}
	if *static != "" {
		if info, e := os.Stat(filepath.Join(*static, "index.html")); e != nil || info.IsDir() {
			slog.Error("built frontend index.html is missing")
			os.Exit(1)
		}
	}
	server := &http.Server{Addr: *addr, Handler: httpapi.NewWithOptions(store, strings.Split(*origin, ","), httpapi.Options{Engine: engine, StaticDir: *static}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	failed := make(chan error, 1)
	go func() {
		slog.Info("research workbench started", "addr", *addr, "article_engine_ready", false, "r08_ready", engine != nil)
		failed <- server.ListenAndServe()
	}()
	select {
	case err := <-failed:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			slog.Error("shutdown failed", "error", err)
		}
	}
}
