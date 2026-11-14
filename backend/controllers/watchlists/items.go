package watchlists

import (
	"net/http"
	"strings"
	"time"

	"backend/configs/db"
	"backend/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// AddItem adds one already-identified instrument (picked from search results).
func (ct *Controller) AddItem(c *gin.Context) {
	id, ok := objectID(c)
	if !ok {
		return
	}

	var req struct {
		Symbol   string `json:"symbol"`
		Name     string `json:"name"`
		Exchange string `json:"exchange"`
		Type     string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Symbol) == "" {
		fail(c, http.StatusBadRequest, "symbol is required")
		return
	}

	ctx := c.Request.Context()
	added, err := addItem(ctx, id, models.ListItem{
		Symbol:   strings.TrimSpace(req.Symbol),
		Name:     strings.TrimSpace(req.Name),
		Exchange: strings.TrimSpace(req.Exchange),
		Type:     strings.TrimSpace(req.Type),
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not add item")
		return
	}
	if !added {
		exists, err := db.Lists().CountDocuments(ctx, bson.M{"_id": id})
		if err != nil {
			fail(c, http.StatusInternalServerError, "could not add item")
			return
		}
		if exists == 0 {
			fail(c, http.StatusNotFound, "list not found")
			return
		}
		fail(c, http.StatusConflict, "already in this list, or the list is full")
		return
	}

	ct.Kick() // fetch its price now
	c.Status(http.StatusNoContent)
}

// RemoveItem drops one instrument; identified by ?symbol=&exchange= because symbols can contain "/".
func (ct *Controller) RemoveItem(c *gin.Context) {
	id, ok := objectID(c)
	if !ok {
		return
	}

	symbol := c.Query("symbol")
	if symbol == "" {
		fail(c, http.StatusBadRequest, "symbol is required")
		return
	}

	res, err := db.Lists().UpdateOne(c.Request.Context(), bson.M{"_id": id}, bson.M{
		"$pull": bson.M{"items": bson.M{"symbol": symbol, "exchange": c.Query("exchange")}},
		"$set":  bson.M{"updatedAt": time.Now()},
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not remove item")
		return
	}
	if res.MatchedCount == 0 {
		fail(c, http.StatusNotFound, "list not found")
		return
	}
	c.Status(http.StatusNoContent)
}

// AddEntries queues comma-separated symbols and/or names; the worker resolves them one by one.
func (ct *Controller) AddEntries(c *gin.Context) {
	id, ok := objectID(c)
	if !ok {
		return
	}

	var req struct {
		Entries string `json:"entries"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	entries, err := parseEntries(req.Entries)
	if err != nil || len(entries) == 0 {
		msg := "enter at least one symbol or name"
		if err != nil {
			msg = err.Error()
		}
		fail(c, http.StatusBadRequest, msg)
		return
	}

	res, err := db.Lists().UpdateOne(c.Request.Context(), bson.M{"_id": id}, bson.M{
		"$push": bson.M{"pending": bson.M{"$each": entries}},
		"$set":  bson.M{"updatedAt": time.Now()},
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not queue entries")
		return
	}
	if res.MatchedCount == 0 {
		fail(c, http.StatusNotFound, "list not found")
		return
	}

	ct.Kick()
	c.Status(http.StatusAccepted)
}
