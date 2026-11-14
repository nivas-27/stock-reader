package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ListItem is one instrument inside a list.
type ListItem struct {
	Symbol   string `bson:"symbol"`
	Name     string `bson:"name"`
	Exchange string `bson:"exchange"` // empty for forex/crypto pairs
	Type     string `bson:"type"`     // e.g. "Common Stock", "ETF", "Digital Currency"
}

// List is one user-created watchlist (collection: lists).
type List struct {
	ID         bson.ObjectID `bson:"_id,omitempty"`
	Name       string        `bson:"name"`
	Items      []ListItem    `bson:"items"`
	Pending    []string      `bson:"pending"`    // raw entries still waiting to be resolved
	Unresolved []string      `bson:"unresolved"` // entries that matched nothing
	CreatedAt  time.Time     `bson:"createdAt"`
	UpdatedAt  time.Time     `bson:"updatedAt"`
}

// Quote is one instrument's analysed day, fetched once per day (collection: quotes).
type Quote struct {
	Key       string    `bson:"key"` // symbol|exchange
	Day       string    `bson:"day"` // YYYY-MM-DD
	Price     float64   `bson:"price"`
	Change    float64   `bson:"change"`
	ChangePct float64   `bson:"changePct"`
	Trend     string    `bson:"trend"` // Uptrend, Downtrend or Sideways
	Spark     []float64 `bson:"spark"`
	Error     string    `bson:"error,omitempty"`
	FetchedAt time.Time `bson:"fetchedAt"`
}

// SymbolMatch is one symbol_search hit.
type SymbolMatch struct {
	Symbol   string `bson:"symbol"`
	Name     string `bson:"name"`
	Exchange string `bson:"exchange"`
	Type     string `bson:"type"`
	Country  string `bson:"country"`
	Currency string `bson:"currency"`
}

// SearchCache stores one search term's results to save API credits (collection: search_cache).
type SearchCache struct {
	Query     string        `bson:"_id"` // lower-cased term
	Results   []SymbolMatch `bson:"results"`
	FetchedAt time.Time     `bson:"fetchedAt"`
}
