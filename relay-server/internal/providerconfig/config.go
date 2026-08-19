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
// host/port/mountpoint/username/password are the fields every provider
// in rtk_provider_comparison.md requires; the rest capture the
// per-provider differences that showed up in that research (NTRIP
// version, TLS port, GGA cadence, arbitrary extra headers) so a real
// provider's quirks don't require a code change to onboard.
type Provider struct {
	Host       string `json:"host"`
	Port       string `json:"port"`
	Mountpoint string `json:"mountpoint"`
	Username   string `json:"username"`
	Password   string `json:"password"`

	// NTRIPVersion is "1" or "2". Empty defaults to "2". docomo, for
	// example, exposes both a plain (2101) and TLS (2102) port; other
	// providers only document one NTRIP version.
	NTRIPVersion string `json:"ntrip_version,omitempty"`
	// TLS dials the provider over TLS instead of plain TCP.
	TLS bool `json:"tls,omitempty"`
	// GGAIntervalSeconds is the provider's recommended/required GGA
	// send interval (e.g. ichimill recommends 1s). Advisory only - the
	// relay doesn't enforce it since it passes GGA through verbatim.
	GGAIntervalSeconds int `json:"gga_interval_seconds,omitempty"`
	// ExtraHeaders are sent verbatim on the NTRIP request to the
	// provider, for whatever provider-specific parameter isn't covered
	// above (e.g. an account/contract ID header some providers require
	// in addition to the NTRIP username/password).
	ExtraHeaders map[string]string `json:"extra_headers,omitempty"`
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
