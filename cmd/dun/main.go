// Command dun is the Dun HTTP data plane.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NICE-DEV226/Dun/internal/config"
	"github.com/NICE-DEV226/Dun/internal/extension"
	dunhttp "github.com/NICE-DEV226/Dun/internal/http"
	"github.com/NICE-DEV226/Dun/internal/runtime/wazero"
	"github.com/NICE-DEV226/Dun/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "dun: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("dun", flag.ExitOnError)
	cfgPath := fs.String("config", "dun.yaml", "path to configuration file")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: dun [command] [flags]\n\nCommands:\n  run       start the data plane\n  check     validate configuration\n  version   print version\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	args := fs.Args()
	command := "run"
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
		if err := fs.Parse(args); err != nil {
			return err
		}
	}

	switch command {
	case "run":
		return runServer(*cfgPath)
	case "check":
		return checkConfig(*cfgPath)
	case "version":
		fmt.Println(version.String())
		return nil
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func runServer(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	logger := newLogger(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s, err := dunhttp.New(cfg, dunhttp.WithLogger(logger))
	if err != nil {
		return err
	}

	// Load plugin wasm artifacts and wire per-route pipelines.
	if len(cfg.Plugins) > 0 {
		rt, err := wazero.NewRuntime(logger)
		if err != nil {
			return fmt.Errorf("init wasm runtime: %w", err)
		}
		defer rt.Close(context.Background())

		mgr := extension.NewManager(rt, logger)
		defer func() {
			if err := mgr.CloseAll(context.Background()); err != nil {
				logger.Error("failed to close plugins", "error", err)
			}
		}()
		for _, p := range cfg.Plugins {
			caps := extension.NewSet(p.Capabilities...)
			mgr.SetPolicy(p.Name, caps)
		}
		if err := s.LoadPlugins(ctx, mgr); err != nil {
			return fmt.Errorf("load plugins: %w", err)
		}

		// Hot-reload: poll wasm artifacts for changes and atomic-swap
		// new versions (RFC-0000 §13).
		if len(cfg.Plugins) > 0 {
			w := s.WatchPlugins(ctx, 5*time.Second)
			if w != nil {
				logger.Info("plugin watcher started", "paths", w.Paths(), "interval", "5s")
			}
		}
	}

	return s.ListenAndServe(ctx)
}

func checkConfig(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	pluginInfo := ""
	if len(cfg.Plugins) > 0 {
		pluginInfo = fmt.Sprintf(", %d plugin(s)", len(cfg.Plugins))
	}
	fmt.Printf("configuration OK: %d route(s), listening on %s%s\n", len(cfg.Routes), cfg.Listen.Address, pluginInfo)
	return nil
}

func newLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	if cfg.Log.Format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
