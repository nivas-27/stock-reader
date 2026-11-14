package watchlists

import (
	"context"
	"errors"
	"log"
	"time"

	"backend/configs/db"
	"backend/models"
	"backend/utils/twelvedata"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	passEvery  = 30 * time.Minute
	retryAfter = time.Minute
)

// runWorker resolves typed entries and fetches each listed instrument's daily
// data once per day. It runs on start, every 30 minutes, and whenever kicked.
func (ct *Controller) runWorker() {
	ticker := time.NewTicker(passEvery)
	defer ticker.Stop()

	for {
		ct.pass()
		select {
		case <-ticker.C:
		case <-ct.kick:
		}
	}
}

func (ct *Controller) pass() {
	if !ct.client.HasKey() {
		return
	}
	ctx := context.Background()

	for {
		progressed, err := ct.step(ctx)

		var limit *twelvedata.RateLimitError
		switch {
		case err == nil && progressed:
			continue
		case err == nil:
			return
		case errors.As(err, &limit) && !limit.Daily:
			log.Println("watchlists: minute limit hit, pausing a minute")
			time.Sleep(retryAfter)
		case errors.As(err, &limit):
			log.Println("watchlists: daily credit limit reached, will retry later")
			return
		case errors.Is(err, twelvedata.ErrBadKey):
			log.Println("watchlists:", err)
			return
		default:
			log.Println("watchlists: pass failed, retrying shortly:", err)
			ct.kickAfter(retryAfter)
			return
		}
	}
}

// step does one unit of work: resolve a typed entry first, else fetch one missing quote.
func (ct *Controller) step(ctx context.Context) (bool, error) {
	if done, err := ct.resolveOne(ctx); done || err != nil {
		return done, err
	}
	return ct.fetchOne(ctx)
}

func (ct *Controller) resolveOne(ctx context.Context) (bool, error) {
	var list models.List
	err := db.Lists().FindOne(ctx, bson.M{"pending.0": bson.M{"$exists": true}},
		options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: 1}})).Decode(&list)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	entry := list.Pending[0]
	item, found, err := ct.resolveEntry(ctx, entry)
	var api *twelvedata.APIError
	if errors.As(err, &api) {
		found, err = false, nil // Twelve Data refused this term: report it as unresolved
	}
	if err != nil {
		return false, err
	}

	note := ""
	switch {
	case !found:
		note = entry
	default:
		added, err := addItem(ctx, list.ID, item)
		if err != nil {
			return false, err
		}
		if !added {
			if dup, err := hasItem(ctx, list.ID, item); err != nil {
				return false, err
			} else if !dup {
				note = entry + " (list is full)"
			}
		}
	}

	update := bson.M{"$pop": bson.M{"pending": -1}, "$set": bson.M{"updatedAt": time.Now()}}
	if note != "" {
		update["$push"] = bson.M{"unresolved": note}
	}
	if _, err := db.Lists().UpdateOne(ctx, bson.M{"_id": list.ID}, update); err != nil {
		return false, err
	}
	return true, nil
}

func (ct *Controller) fetchOne(ctx context.Context) (bool, error) {
	day := today()
	it, ok, err := nextMissing(ctx, day)
	if err != nil || !ok {
		return false, err
	}

	key := itemKey(it.Symbol, it.Exchange)
	closes, err := ct.client.TimeSeries(ctx, it.Symbol, fetchExchange(it))

	var q models.Quote
	var api *twelvedata.APIError
	switch {
	case errors.As(err, &api):
		q = models.Quote{Key: key, Day: day, FetchedAt: time.Now(), Error: api.Message}
	case err != nil:
		return false, err
	default:
		built, berr := buildQuote(key, day, closes)
		if berr != nil {
			built = models.Quote{Key: key, Day: day, FetchedAt: time.Now(), Error: berr.Error()}
		}
		q = built
	}

	if err := saveQuote(ctx, q); err != nil {
		return false, err
	}
	return true, nil
}

// Pairs trade everywhere; naming an exchange for them can only get in the way.
func fetchExchange(it models.ListItem) string {
	if it.Type == "Physical Currency" || it.Type == "Digital Currency" {
		return ""
	}
	return it.Exchange
}
