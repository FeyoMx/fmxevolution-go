package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/auth"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"gorm.io/gorm"
)

const (
	defaultTenantName = "FMX"
	defaultTenantSlug = "fmx"
	defaultOwnerEmail = "contacto@fmxaiflows.online"
	defaultOwnerPass  = "admin123"
	defaultOwnerName  = "FMX Owner"
)

func SeedDefaultTenant(ctx context.Context, stores *repository.Stores, logger *slog.Logger) error {
	if stores == nil || stores.DB == nil {
		return fmt.Errorf("bootstrap seed requires initialized stores")
	}
	if logger == nil {
		logger = slog.Default()
	}

	result, err := runDefaultSeed(ctx, stores.DB)
	if err != nil {
		return err
	}

	logger.Info("bootstrap seed completed",
		"tenant_slug", defaultTenantSlug,
		"tenant_created", result.tenantCreated,
		"user_created", result.userCreated,
		"user_role", auth.RoleOwner,
		"user_email", defaultOwnerEmail,
	)
	return nil
}

func ResetDefaultAdminPassword(ctx context.Context, stores *repository.Stores, appEnv string, logger *slog.Logger) error {
	if !shouldResetAdminPassword(appEnv) {
		return nil
	}
	if stores == nil || stores.DB == nil {
		return fmt.Errorf("admin password reset requires initialized stores")
	}
	if logger == nil {
		logger = slog.Default()
	}

	err := stores.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant repository.Tenant
		if err := tx.First(&tenant, "slug = ?", defaultTenantSlug).Error; err != nil {
			return err
		}

		var user repository.User
		email := strings.ToLower(strings.TrimSpace(defaultOwnerEmail))
		if err := tx.First(&user, "tenant_id = ? AND email = ?", tenant.ID, email).Error; err != nil {
			return err
		}

		passwordHash, err := repository.HashPassword(defaultOwnerPass)
		if err != nil {
			return fmt.Errorf("hash admin reset password: %w", err)
		}

		return tx.Model(&repository.User{}).
			Where("id = ?", user.ID).
			Update("password_hash", passwordHash).Error
	})
	if err != nil {
		return err
	}

	logger.Info("admin password reset", "user_email", defaultOwnerEmail, "tenant_slug", defaultTenantSlug)
	return nil
}

type seedResult struct {
	tenantCreated bool
	userCreated   bool
}

func runDefaultSeed(ctx context.Context, db *gorm.DB) (seedResult, error) {
	result := seedResult{}

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant repository.Tenant
		err := tx.First(&tenant, "slug = ?", defaultTenantSlug).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			createdTenant, createErr := createDefaultTenant()
			if createErr != nil {
				return createErr
			}
			if err := tx.Create(createdTenant).Error; err != nil {
				return err
			}
			tenant = *createdTenant
			result.tenantCreated = true
		case err != nil:
			return err
		}

		var user repository.User
		email := strings.ToLower(strings.TrimSpace(defaultOwnerEmail))
		err = tx.First(&user, "tenant_id = ? AND email = ?", tenant.ID, email).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			passwordHash, hashErr := repository.HashPassword(defaultOwnerPass)
			if hashErr != nil {
				return fmt.Errorf("hash bootstrap password: %w", hashErr)
			}

			createdUser := &repository.User{
				TenantID:     tenant.ID,
				Email:        email,
				PasswordHash: passwordHash,
				Name:         defaultOwnerName,
				Role:         auth.RoleOwner,
			}
			if err := tx.Create(createdUser).Error; err != nil {
				return err
			}
			result.userCreated = true
		case err != nil:
			return err
		}

		return nil
	})

	return result, err
}

func createDefaultTenant() (*repository.Tenant, error) {
	rawAPIKey, err := repository.GenerateTenantAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generate bootstrap api key: %w", err)
	}
	apiKeyHash, err := repository.HashAPIKey(rawAPIKey)
	if err != nil {
		return nil, fmt.Errorf("hash bootstrap api key: %w", err)
	}

	return &repository.Tenant{
		Name:         defaultTenantName,
		Slug:         defaultTenantSlug,
		APIKeyPrefix: repository.APIKeyPrefix(rawAPIKey),
		APIKeyHash:   apiKeyHash,
	}, nil
}

func shouldResetAdminPassword(appEnv string) bool {
	if strings.EqualFold(strings.TrimSpace(appEnv), "production") {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(os.Getenv("RESET_ADMIN_PASSWORD")), "true")
}
