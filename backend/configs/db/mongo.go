package db

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	defaultURI = "mongodb://localhost:27017"
	defaultDB  = "nivas_stock_reader"

	quoteTTL  = 14 * 24 * time.Hour
	searchTTL = 7 * 24 * time.Hour
)

var (
	client   *mongo.Client
	database *mongo.Database
)

// Connect opens the connection (MONGO_URI / MONGO_DB override the defaults),
// checks it, and makes sure the collections' indexes exist.
func Connect(ctx context.Context) error {
	uri := envOr("MONGO_URI", defaultURI)
	name := envOr("MONGO_DB", defaultDB)

	c, err := mongo.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(5 * time.Second))
	if err != nil {
		return fmt.Errorf("mongo connect: %w", err)
	}
	if err := c.Ping(ctx, nil); err != nil {
		_ = c.Disconnect(ctx)
		return fmt.Errorf("mongo ping %s: %w", uri, err)
	}

	client = c
	database = c.Database(name)
	return ensureIndexes(ctx)
}

func Disconnect(ctx context.Context) {
	if client != nil {
		_ = client.Disconnect(ctx)
	}
}

func Lists() *mongo.Collection       { return database.Collection("lists") }
func Quotes() *mongo.Collection      { return database.Collection("quotes") }
func SearchCache() *mongo.Collection { return database.Collection("search_cache") }

func ensureIndexes(ctx context.Context) error {
	if _, err := Lists().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "createdAt", Value: 1}},
	}); err != nil {
		return fmt.Errorf("lists index: %w", err)
	}

	if _, err := Quotes().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "key", Value: 1}, {Key: "day", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "fetchedAt", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(int32(quoteTTL.Seconds())),
		},
	}); err != nil {
		return fmt.Errorf("quotes indexes: %w", err)
	}

	if _, err := SearchCache().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "fetchedAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(int32(searchTTL.Seconds())),
	}); err != nil {
		return fmt.Errorf("search_cache index: %w", err)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
