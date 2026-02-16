// Command miner is the entry point for the Twitch Channel Points Miner.
// It loads account configurations, starts one Miner per account, and
// manages graceful shutdown via OS signals.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/config"
	"github.com/Guliveer/twitch-miner-go/internal/logger"
	"github.com/Guliveer/twitch-miner-go/internal/miner"
	"github.com/Guliveer/twitch-miner-go/internal/model"
	"github.com/Guliveer/twitch-miner-go/internal/server"
	"golang.org/x/term"
)

const banner = `
╔══════════════════════════════════════════════════╗
║     Twitch Channel Points Miner — Go Edition     ║
╚══════════════════════════════════════════════════╝
`

func main() {
	configDir := flag.String("config", "configs", "Path to the configuration directory")
	port := flag.String("port", "8080", "Port for the health/analytics HTTP server")
	logLevel := flag.String("log-level", "", "Log level: DEBUG, INFO, WARN, ERROR (overrides LOG_LEVEL env)")
	noColor := flag.Bool("no-color", false, "Disable colored output (overrides TTY detection)")
	flag.Parse()

	level := slog.LevelInfo
	if *logLevel != "" {
		level = logger.ParseLevel(*logLevel)
	} else if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		level = logger.ParseLevel(envLevel)
	}

	httpPort := *port
	if envPort := os.Getenv("PORT"); envPort != "" {
		httpPort = envPort
	}

	colored := !*noColor && term.IsTerminal(int(os.Stdout.Fd())) && os.Getenv("NO_COLOR") == ""

	rootLog, err := logger.Setup(logger.Config{
		Level:   level,
		Colored: colored,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup logger: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(banner)
	rootLog.Info("🚀 Starting Twitch Channel Points Miner (Go)")

	configs, err := config.LoadAllAccountConfigs(*configDir)
	if err != nil {
		rootLog.Error("Failed to load account configs", "dir", *configDir, "error", err)
		os.Exit(1)
	}

	for _, cfg := range configs {
		if err := config.Validate(cfg); err != nil {
			rootLog.Error("Invalid config", "account", cfg.Username, "error", err)
			os.Exit(1)
		}
	}

	rootLog.Info("📂 Loaded account configurations",
		"count", len(configs),
		"config_dir", *configDir,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		rootLog.Info("Received shutdown signal", "signal", sig.String())
		cancel()

		time.AfterFunc(30*time.Second, func() {
			rootLog.Error("Graceful shutdown timed out, forcing exit")
			os.Exit(1)
		})
	}()

	miners := make([]*miner.Miner, 0, len(configs))
	for _, cfg := range configs {
		if !cfg.IsEnabled() {
			rootLog.Info("Account is disabled, skipping", "account", cfg.Username)
			continue
		}
		accountLog := rootLog.WithAccount(cfg.Username)
		minerInstance := miner.NewMiner(cfg, accountLog)
		miners = append(miners, minerInstance)
	}

	addr := ":" + httpPort
	analyticsServer := server.NewAnalyticsServer(addr, rootLog)

	analyticsServer.SetStreamerFunc(func() []*model.Streamer {
		var all []*model.Streamer
		for _, minerInstance := range miners {
			all = append(all, minerInstance.Streamers()...)
		}
		return all
	})

	go func() {
		if err := analyticsServer.Run(ctx); err != nil && ctx.Err() == nil {
			rootLog.Error("Analytics server failed", "error", err)
		}
	}()

	rootLog.Info("🌐 Health/analytics server started", "addr", addr)

	var wg sync.WaitGroup
	for i, minerInstance := range miners {
		cfg := configs[i]
		accountLog := rootLog.WithAccount(cfg.Username)

		wg.Add(1)
		go func(minerInstance *miner.Miner) {
			defer wg.Done()
			if err := minerInstance.Run(ctx); err != nil {
				if ctx.Err() != nil {
					accountLog.Info("Miner stopped due to shutdown", "account", cfg.Username)
				} else {
					accountLog.Error("Miner failed", "account", cfg.Username, "error", err)
				}
			}
		}(minerInstance)
	}

	wg.Wait()

	if ctx.Err() != nil {
		rootLog.Info("🛑 Shutdown complete")
	}

	rootLog.Info("👋 All miners stopped. Goodbye!")
}
