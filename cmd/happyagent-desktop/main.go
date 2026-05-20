package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"happyagent/internal/career"
	"happyagent/internal/config"
	"happyagent/internal/desktop"
	"happyagent/internal/runtime"
)

func main() {
	var addr string
	var workspaceRoot string
	var staticDir string
	flag.StringVar(&addr, "addr", "127.0.0.1:0", "listen address")
	flag.StringVar(&workspaceRoot, "workspace", career.DefaultWorkspaceRoot, "workspace root")
	flag.StringVar(&staticDir, "static", filepath.Join("desktop", "dist"), "static frontend directory")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		exitf("load config: %v", err)
	}
	workspaceRoot = prepareDesktopConfig(&cfg, workspaceRoot)
	if err := ensureDesktopWorkspace(workspaceRoot); err != nil {
		exitf("prepare workspace: %v", err)
	}

	rt, err := runtime.NewBuilder().Build(cfg)
	if err != nil {
		exitf("build runtime: %v", err)
	}
	defer rt.Close()

	server, err := desktop.NewServer(desktop.Options{
		Config:        cfg,
		Runtime:       rt,
		WorkspaceRoot: workspaceRoot,
		StaticDir:     staticDir,
	})
	if err != nil {
		exitf("build desktop server: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	url, err := server.ListenAndServe(ctx, addr)
	if err != nil {
		exitf("listen: %v", err)
	}
	fmt.Fprintf(os.Stdout, "happyagent desktop ready: %s\n", url)
	<-ctx.Done()
}

func prepareDesktopConfig(cfg *config.Config, workspaceRoot string) string {
	career.PrepareConfig(cfg, []string{"career"})
	if workspaceRoot == "" {
		workspaceRoot = career.DefaultWorkspaceRoot
	}
	cfg.Tools.RootDir = workspaceRoot
	return workspaceRoot
}

func ensureDesktopWorkspace(workspaceRoot string) error {
	_, err := career.OpenWorkspace(workspaceRoot, time.Now())
	return err
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
