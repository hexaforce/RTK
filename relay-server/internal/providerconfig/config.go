// Package providerconfig loads the upstream RTK provider's NTRIP
// connection details, abstracting away which provider is behind it
// (see rtk_relay_server_requirements.md, "Provider Adapter").
package providerconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// Provider holds the connection details for one upstream NTRIP Caster.
type Provider struct {
	Host       string `json:"host"`
	Port       string `json:"port"`
	Mountpoint string `json:"mountpoint"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

func (p Provider) Addr() string {
	return fmt.Sprintf("%s:%s", p.Host, p.Port)
}

// LoadFromEnv builds a Provider from discrete PROVIDER_* env vars.
// Used for local development where no Secrets Manager is available.
func LoadFromEnv() (Provider, error) {
	p := Provider{
		Host:       os.Getenv("PROVIDER_HOST"),
		Port:       os.Getenv("PROVIDER_PORT"),
		Mountpoint: os.Getenv("PROVIDER_MOUNTPOINT"),
		Username:   os.Getenv("PROVIDER_USERNAME"),
		Password:   os.Getenv("PROVIDER_PASSWORD"),
	}
	if p.Host == "" || p.Port == "" || p.Mountpoint == "" {
		return Provider{}, fmt.Errorf("PROVIDER_HOST, PROVIDER_PORT and PROVIDER_MOUNTPOINT must be set")
	}
	return p, nil
}

// LoadFromSecretsManager fetches and decodes a Provider from a Secrets
// Manager secret (JSON matching the Provider fields).
func LoadFromSecretsManager(ctx context.Context, client *secretsmanager.Client, secretARN string) (Provider, error) {
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &secretARN,
	})
	if err != nil {
		return Provider{}, fmt.Errorf("get secret value: %w", err)
	}
	if out.SecretString == nil {
		return Provider{}, fmt.Errorf("secret %s has no string value", secretARN)
	}
	var p Provider
	if err := json.Unmarshal([]byte(*out.SecretString), &p); err != nil {
		return Provider{}, fmt.Errorf("decode secret json: %w", err)
	}
	if p.Host == "" || p.Port == "" || p.Mountpoint == "" {
		return Provider{}, fmt.Errorf("secret %s is missing host/port/mountpoint", secretARN)
	}
	return p, nil
}
