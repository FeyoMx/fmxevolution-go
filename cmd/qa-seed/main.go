package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/config"
	"github.com/EvolutionAPI/evolution-go/internal/qaseed"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	pkglogger "github.com/EvolutionAPI/evolution-go/pkg/logger"
	"github.com/joho/godotenv"
)

func main() {
	var opts qaseed.Options
	var timeout time.Duration

	flag.StringVar(&opts.TenantID, "tenant-id", "", "existing tenant id to seed")
	flag.StringVar(&opts.TenantSlug, "tenant-slug", "qa-seed", "tenant slug to seed or create")
	flag.BoolVar(&opts.CreateTenant, "create-tenant", true, "create the QA tenant/admin if the slug does not exist")
	flag.StringVar(&opts.AdminEmail, "admin-email", "qa.admin@example.test", "admin email used when creating or repairing the QA tenant")
	flag.StringVar(&opts.AdminPassword, "admin-password", "QaSeed123!", "admin password used only when creating the QA admin")
	flag.DurationVar(&timeout, "timeout", 2*time.Minute, "maximum seed runtime")
	flag.Parse()

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
	logger := pkglogger.NewStructuredLogger(cfg.AppEnv).Logger().With("module", "qa_seed")

	stores, err := repository.NewStores(
		cfg.Database.URL,
		cfg.Database.MaxOpenConns,
		cfg.Database.MaxIdleConns,
		cfg.Database.ConnMaxLifetime,
	)
	if err != nil {
		log.Fatalf("open stores: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		if err := repository.Close(closeCtx, stores); err != nil {
			logger.Warn("close stores after qa seed", "error", err)
		}
	}()

	summary, err := qaseed.Run(ctx, cfg, stores, logger, opts)
	if err != nil {
		log.Fatalf("qa seed failed: %v", err)
	}

	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		log.Fatalf("encode summary: %v", err)
	}
	fmt.Println(string(encoded))
}
