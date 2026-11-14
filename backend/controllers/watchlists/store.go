package watchlists

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"backend/configs/db"
	"backend/models"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// loadQuotes returns the day's saved quotes for the given keys.
func loadQuotes(ctx context.Context, day string, keys []string) (map[string]models.Quote, error) {
	out := map[string]models.Quote{}
	if len(keys) == 0 {
		return out, nil
	}

	cur, err := db.Quotes().Find(ctx, bson.M{"day": day, "key": bson.M{"$in": keys}})
	if err != nil {
		return nil, err
	}
	var quotes []models.Quote
	if err := cur.All(ctx, &quotes); err != nil {
		return nil, err
	}
	for _, q := range quotes {
		out[q.Key] = q
	}
	return out, nil
}

func saveQuote(ctx context.Context, q models.Quote) error {
	_, err := db.Quotes().ReplaceOne(ctx, bson.M{"key": q.Key, "day": q.Day}, q, options.Replace().SetUpsert(true))
	return err
}

// nextMissing returns the first listed instrument with no quote for the day.
func nextMissing(ctx context.Context, day string) (models.ListItem, bool, error) {
	cur, err := db.Lists().Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetProjection(bson.M{"items": 1}))
	if err != nil {
		return models.ListItem{}, false, err
	}
	var lists []models.List
	if err := cur.All(ctx, &lists); err != nil {
		return models.ListItem{}, false, err
	}

	var ordered []models.ListItem
	var keys []string
	seen := map[string]bool{}
	for _, l := range lists {
		for _, it := range l.Items {
			k := itemKey(it.Symbol, it.Exchange)
			if !seen[k] {
				seen[k] = true
				ordered = append(ordered, it)
				keys = append(keys, k)
			}
		}
	}

	have, err := loadQuotes(ctx, day, keys)
	if err != nil {
		return models.ListItem{}, false, err
	}
	for _, it := range ordered {
		if _, ok := have[itemKey(it.Symbol, it.Exchange)]; !ok {
			return it, true, nil
		}
	}
	return models.ListItem{}, false, nil
}

// addItem pushes the item unless the list is missing, full, or already holds it.
func addItem(ctx context.Context, id bson.ObjectID, it models.ListItem) (bool, error) {
	filter := bson.M{
		"_id":   id,
		"items": bson.M{"$not": bson.M{"$elemMatch": bson.M{"symbol": it.Symbol, "exchange": it.Exchange}}},
		fmt.Sprintf("items.%d", maxItemsPerList-1): bson.M{"$exists": false},
	}
	res, err := db.Lists().UpdateOne(ctx, filter, bson.M{
		"$push": bson.M{"items": it},
		"$set":  bson.M{"updatedAt": time.Now()},
	})
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

func hasItem(ctx context.Context, id bson.ObjectID, it models.ListItem) (bool, error) {
	n, err := db.Lists().CountDocuments(ctx, bson.M{
		"_id":   id,
		"items": bson.M{"$elemMatch": bson.M{"symbol": it.Symbol, "exchange": it.Exchange}},
	})
	return n > 0, err
}

// searchCached answers from search_cache when it can, so each term costs one credit at most once.
func (ct *Controller) searchCached(ctx context.Context, term string, interactive bool) ([]models.SymbolMatch, error) {
	key := strings.ToLower(strings.TrimSpace(term))

	var cached models.SearchCache
	err := db.SearchCache().FindOne(ctx, bson.M{"_id": key}).Decode(&cached)
	if err == nil {
		return cached.Results, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}

	results, err := ct.client.SymbolSearch(ctx, term, interactive)
	if err != nil {
		return nil, err
	}

	entry := models.SearchCache{Query: key, Results: results, FetchedAt: time.Now()}
	_, _ = db.SearchCache().ReplaceOne(ctx, bson.M{"_id": key}, entry, options.Replace().SetUpsert(true))
	return results, nil
}
