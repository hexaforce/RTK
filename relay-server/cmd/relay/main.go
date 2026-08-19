package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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

	providers, vehicleAuth := mustLoadDependencies(ctx, logger)

	srv := &relay.Server{
		ListenAddr: listenAddr,
		Providers:  providers,
		Auth:       vehicleAuth,
		Logger:     logger,
	}

	if err := srv.ListenAndServe(ctx); err != nil {
		logger.Error("relay server exited with error", "err", err)
		os.Exit(1)
	}
}

// mustLoadDependencies wires up the vehicle authenticator and provider
// resolver. Each is independently AWS-backed or local-fallback:
//   - VEHICLE_TABLE_NAME set -> DynamoDB vehicle auth (each item's
//     provider_id says which provider that vehicle uses).
//   - PROVIDER_SECRET_PREFIX set -> Secrets Manager provider resolver,
//     one secret per provider named "<prefix><provider_id>". Adding a
//     provider is then just creating a new secret, no redeploy needed.
//   - Otherwise: local fallback (VEHICLE_API_KEYS / PROVIDER_HOST etc.)
//     that ignores per-vehicle provider assignment - local dev only
//     ever exercises one provider at a time.
func mustLoadDependencies(ctx context.Context, logger *slog.Logger) (providerconfig.Resolver, auth.VehicleAuthenticator) {
	secretPrefix := os.Getenv("PROVIDER_SECRET_PREFIX")
	tableName := os.Getenv("VEHICLE_TABLE_NAME")

	var providers providerconfig.Resolver
	var vehicleAuth auth.VehicleAuthenticator

	if secretPrefix != "" || tableName != "" {
		cfg, err := config.LoadDefaultConfig(ctx, config.WithRetryMaxAttempts(3))
		if err != nil {
			logger.Error("failed to load AWS config", "err", err)
			os.Exit(1)
		}

		if secretPrefix != "" {
			smClient := secretsmanager.NewFromConfig(cfg)
			providers = providerconfig.SecretsManagerResolver{Client: smClient, Prefix: secretPrefix}
		}

		if tableName != "" {
			ddbClient := dynamodb.NewFromConfig(cfg)
			vehicleAuth = auth.NewDynamoDBAuthenticator(ddbClient, tableName)
		}
	}

	if secretPrefix == "" {
		provider, err := providerconfig.LoadFromEnv()
		if err != nil {
			logger.Error("failed to load provider config from env", "err", err)
			os.Exit(1)
		}
		providers = providerconfig.StaticResolver{Provider: provider}
	}

	if tableName == "" {
		vehicleAuth = auth.NewStaticAuthenticatorFromEnv()
	}

	return providers, vehicleAuth
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
