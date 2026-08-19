package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/fpv-japan/rtk-relay/internal/auth"
	"github.com/fpv-japan/rtk-relay/internal/providerconfig"
	"github.com/fpv-japan/rtk-relay/internal/relay"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	listenAddr := envOr("LISTEN_ADDR", ":2101")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	provider, vehicleAuth := mustLoadDependencies(ctx, logger)

	srv := &relay.Server{
		ListenAddr: listenAddr,
		Provider:   provider,
		Auth:       vehicleAuth,
		Logger:     logger,
	}

	if err := srv.ListenAndServe(ctx); err != nil {
		logger.Error("relay server exited with error", "err", err)
		os.Exit(1)
	}
}

func mustLoadDependencies(ctx context.Context, logger *slog.Logger) (providerconfig.Provider, auth.VehicleAuthenticator) {
	secretARN := os.Getenv("PROVIDER_SECRET_ARN")
	tableName := os.Getenv("VEHICLE_TABLE_NAME")

	var provider providerconfig.Provider
	var vehicleAuth auth.VehicleAuthenticator

	if secretARN != "" || tableName != "" {
		cfg, err := config.LoadDefaultConfig(ctx, config.WithRetryMaxAttempts(3))
		if err != nil {
			logger.Error("failed to load AWS config", "err", err)
			os.Exit(1)
		}

		if secretARN != "" {
			smClient := secretsmanager.NewFromConfig(cfg)
			ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			provider, err = providerconfig.LoadFromSecretsManager(ctx2, smClient, secretARN)
			if err != nil {
				logger.Error("failed to load provider config from Secrets Manager", "err", err)
				os.Exit(1)
			}
		}

		if tableName != "" {
			ddbClient := dynamodb.NewFromConfig(cfg)
			vehicleAuth = auth.NewDynamoDBAuthenticator(ddbClient, tableName)
		}
	}

	if secretARN == "" {
		var err error
		provider, err = providerconfig.LoadFromEnv()
		if err != nil {
			logger.Error("failed to load provider config from env", "err", err)
			os.Exit(1)
		}
	}

	if tableName == "" {
		vehicleAuth = auth.NewStaticAuthenticatorFromEnv()
	}

	return provider, vehicleAuth
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
