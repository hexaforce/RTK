// Package providerconfig loads the upstream RTK provider's NTRIP
// connection details, abstracting away which provider is behind it
// (see rtk_relay_server_requirements.md, "Provider Adapter").
//
// Multiple providers can be registered at once; each vehicle is
// assigned a provider_id (see internal/auth), and the relay resolves
// that vehicle's Provider config per session.
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

// Resolver returns the Provider config a given vehicle (identified by
// its assigned provider_id) should be relayed to.
type Resolver interface {
	Resolve(ctx context.Context, providerID string) (Provider, error)
}

// StaticResolver always returns the same Provider regardless of
// providerID. Used for local development where only one provider is
// configured via env vars and per-vehicle provider assignment isn't
// exercised.
type StaticResolver struct {
	Provider Provider
}

func (r StaticResolver) Resolve(_ context.Context, _ string) (Provider, error) {
	return r.Provider, nil
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

// SecretsManagerResolver resolves a provider_id to a Provider by
// fetching the Secrets Manager secret named "<prefix><providerID>"
// (e.g. prefix "rtk-relay/providers/" + id "docomo-poc"). Adding a new
// provider is then just creating a new secret under that prefix - no
// redeploy or Terraform change required.
type SecretsManagerResolver struct {
	Client *secretsmanager.Client
	Prefix string
}

func (r SecretsManagerResolver) Resolve(ctx context.Context, providerID string) (Provider, error) {
	if providerID == "" {
		return Provider{}, fmt.Errorf("vehicle has no provider_id assigned")
	}
	secretName := r.Prefix + providerID
	out, err := r.Client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &secretName,
	})
	if err != nil {
		return Provider{}, fmt.Errorf("get secret value %s: %w", secretName, err)
	}
	if out.SecretString == nil {
		return Provider{}, fmt.Errorf("secret %s has no string value", secretName)
	}
	var p Provider
	if err := json.Unmarshal([]byte(*out.SecretString), &p); err != nil {
		return Provider{}, fmt.Errorf("decode secret %s json: %w", secretName, err)
	}
	if p.Host == "" || p.Port == "" || p.Mountpoint == "" {
		return Provider{}, fmt.Errorf("secret %s is missing host/port/mountpoint", secretName)
	}
	return p, nil
}
