// Package auth verifies vehicle credentials presented over NTRIP Basic
// auth against a stored API key (hashed) per vehicle ID.
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

// VehicleAuthenticator verifies a vehicleID/apiKey pair.
type VehicleAuthenticator interface {
	Authenticate(ctx context.Context, vehicleID, apiKey string) (bool, error)
}

func hashKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}

// StaticAuthenticator authenticates against an in-memory map. Intended
// for local development (docker-compose) without a DynamoDB dependency.
type StaticAuthenticator struct {
	// vehicleID -> sha256 hex of the expected API key
	keyHashes map[string]string
}

// NewStaticAuthenticatorFromEnv builds a StaticAuthenticator from the
// VEHICLE_API_KEYS env var, formatted as "vehicleID:apiKey,vehicleID:apiKey".
func NewStaticAuthenticatorFromEnv() *StaticAuthenticator {
	raw := os.Getenv("VEHICLE_API_KEYS")
	hashes := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		id, key, ok := strings.Cut(pair, ":")
		if !ok {
			continue
		}
		hashes[id] = hashKey(key)
	}
	return &StaticAuthenticator{keyHashes: hashes}
}

func (s *StaticAuthenticator) Authenticate(_ context.Context, vehicleID, apiKey string) (bool, error) {
	expected, ok := s.keyHashes[vehicleID]
	if !ok {
		return false, nil
	}
	got := hashKey(apiKey)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(got)) == 1, nil
}

// DynamoDBAuthenticator authenticates against a DynamoDB table keyed by
// vehicle_id, storing the SHA-256 hex digest of the API key in the
// api_key_hash attribute.
type DynamoDBAuthenticator struct {
	client    *dynamodb.Client
	tableName string
}

func NewDynamoDBAuthenticator(client *dynamodb.Client, tableName string) *DynamoDBAuthenticator {
	return &DynamoDBAuthenticator{client: client, tableName: tableName}
}

func (d *DynamoDBAuthenticator) Authenticate(ctx context.Context, vehicleID, apiKey string) (bool, error) {
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"vehicle_id": &types.AttributeValueMemberS{Value: vehicleID},
		},
	})
	if err != nil {
		return false, fmt.Errorf("dynamodb get item: %w", err)
	}
	if out.Item == nil {
		return false, nil
	}
	hashAttr, ok := out.Item["api_key_hash"].(*types.AttributeValueMemberS)
	if !ok {
		return false, nil
	}
	got := hashKey(apiKey)
	return subtle.ConstantTimeCompare([]byte(hashAttr.Value), []byte(got)) == 1, nil
}
