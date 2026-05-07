package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/EvolutionAPI/evolution-go/internal/bootstrap"
	"github.com/EvolutionAPI/evolution-go/internal/config"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/EvolutionAPI/evolution-go/internal/server"
	"github.com/EvolutionAPI/evolution-go/internal/service"
	pkglogger "github.com/EvolutionAPI/evolution-go/pkg/logger"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("warning: .env file not found, using process environment")
		} else {
			log.Printf("warning: failed to load .env: %v", err)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	logger := pkglogger.NewStructuredLoggerWithLevel(cfg.AppEnv, cfg.LogLevel).Logger()

	stores, err := repository.NewStores(
		cfg.Database.URL,
		cfg.Database.MaxOpenConns,
		cfg.Database.MaxIdleConns,
		cfg.Database.ConnMaxLifetime,
	)
	if err != nil {
		log.Fatalf("open stores: %v", err)
	}

	if err := bootstrap.SeedDefaultTenant(context.Background(), stores, logger); err != nil {
		log.Fatalf("bootstrap seed: %v", err)
	}
	if err := bootstrap.ResetDefaultAdminPassword(context.Background(), stores, cfg.AppEnv, logger); err != nil {
		log.Fatalf("reset admin password: %v", err)
	}

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	appCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()

	app := service.NewApplication(stores, cfg, logger)
	app.Start(appCtx)

	srv := server.New(cfg, app, logger, stores.SQL)
	shutdownDone := make(chan struct{})

	go func() {
		defer close(shutdownDone)
		<-signalCtx.Done()
		logger.Info("shutdown signal received")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown server", "error", err)
		}
		cancelWorkers()
		if err := app.Stop(shutdownCtx); err != nil {
			logger.Error("stop background workers", "error", err)
		}
		if err := repository.Close(shutdownCtx, stores); err != nil {
			logger.Error("close stores", "error", err)
		}
	}()

	if err := srv.Start(); err != nil {
		log.Fatalf("start server: %v", err)
	}
	if signalCtx.Err() != nil {
		<-shutdownDone
	}
}
