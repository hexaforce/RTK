// Package auth verifies vehicle credentials presented over NTRIP Basic
// auth against a stored API key (hashed) per vehicle ID, and resolves
// which RTK provider that vehicle should be relayed to.
package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Vehicle is a successfully authenticated vehicle.
type Vehicle struct {
	ID         string
	ProviderID string
}

// VehicleAuthenticator verifies a vehicleID/apiKey pair. A nil Vehicle
// with a nil error means the credentials were rejected (not a backend
// failure).
type VehicleAuthenticator interface {
	Authenticate(ctx context.Context, vehicleID, apiKey string) (*Vehicle, error)
}

func hashKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}

type staticEntry struct {
	keyHash    string
	providerID string
}

// StaticAuthenticator authenticates against an in-memory map. Intended
// for local development (docker-compose) without a DynamoDB dependency.
type StaticAuthenticator struct {
	vehicles map[string]staticEntry
}

// NewStaticAuthenticatorFromEnv builds a StaticAuthenticator from the
// VEHICLE_API_KEYS env var, formatted as
// "vehicleID:apiKey:providerID,vehicleID:apiKey:providerID".
func NewStaticAuthenticatorFromEnv() *StaticAuthenticator {
	raw := os.Getenv("VEHICLE_API_KEYS")
	vehicles := make(map[string]staticEntry)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 3)
		if len(parts) != 3 {
			continue
		}
		vehicles[parts[0]] = staticEntry{keyHash: hashKey(parts[1]), providerID: parts[2]}
	}
	return &StaticAuthenticator{vehicles: vehicles}
}

func (s *StaticAuthenticator) Authenticate(_ context.Context, vehicleID, apiKey string) (*Vehicle, error) {
	entry, ok := s.vehicles[vehicleID]
	if !ok {
		return nil, nil
	}
	got := hashKey(apiKey)
	if subtle.ConstantTimeCompare([]byte(entry.keyHash), []byte(got)) != 1 {
		return nil, nil
	}
	return &Vehicle{ID: vehicleID, ProviderID: entry.providerID}, nil
}

// DynamoDBAuthenticator authenticates against a DynamoDB table keyed by
// vehicle_id, storing the SHA-256 hex digest of the API key in the
// api_key_hash attribute and the assigned RTK provider in provider_id.
type DynamoDBAuthenticator struct {
	client    *dynamodb.Client
	tableName string
}

func NewDynamoDBAuthenticator(client *dynamodb.Client, tableName string) *DynamoDBAuthenticator {
	return &DynamoDBAuthenticator{client: client, tableName: tableName}
}

func (d *DynamoDBAuthenticator) Authenticate(ctx context.Context, vehicleID, apiKey string) (*Vehicle, error) {
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"vehicle_id": &types.AttributeValueMemberS{Value: vehicleID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("dynamodb get item: %w", err)
	}
	if out.Item == nil {
		return nil, nil
	}
	hashAttr, ok := out.Item["api_key_hash"].(*types.AttributeValueMemberS)
	if !ok {
		return nil, nil
	}
	got := hashKey(apiKey)
	if subtle.ConstantTimeCompare([]byte(hashAttr.Value), []byte(got)) != 1 {
		return nil, nil
	}
	providerAttr, ok := out.Item["provider_id"].(*types.AttributeValueMemberS)
	if !ok || providerAttr.Value == "" {
		return nil, fmt.Errorf("vehicle %s has no provider_id assigned", vehicleID)
	}
	return &Vehicle{ID: vehicleID, ProviderID: providerAttr.Value}, nil
}
