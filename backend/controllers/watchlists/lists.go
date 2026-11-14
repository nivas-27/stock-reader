package watchlists

import (
	"net/http"
	"time"

	"backend/configs/db"
	"backend/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func objectID(c *gin.Context) (bson.ObjectID, bool) {
	id, err := bson.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid list id")
		return id, false
	}
	return id, true
}

// GetLists returns every list with today's saved quotes merged in. It never calls Twelve Data.
func (ct *Controller) GetLists(c *gin.Context) {
	type itemView struct {
		Symbol    string    `json:"symbol"`
		Name      string    `json:"name"`
		Exchange  string    `json:"exchange"`
		Type      string    `json:"type"`
		Loading   bool      `json:"loading"`
		Price     float64   `json:"price"`
		Change    float64   `json:"change"`
		ChangePct float64   `json:"changePct"`
		Trend     string    `json:"trend"`
		Spark     []float64 `json:"spark"`
		Error     string    `json:"error,omitempty"`
	}
	type listView struct {
		ID         string     `json:"id"`
		Name       string     `json:"name"`
		Items      []itemView `json:"items"`
		Pending    []string   `json:"pending"`
		Unresolved []string   `json:"unresolved"`
	}

	ctx := c.Request.Context()
	cur, err := db.Lists().Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not read lists")
		return
	}
	var lists []models.List
	if err := cur.All(ctx, &lists); err != nil {
		fail(c, http.StatusInternalServerError, "could not read lists")
		return
	}

	var keys []string
	for _, l := range lists {
		for _, it := range l.Items {
			keys = append(keys, itemKey(it.Symbol, it.Exchange))
		}
	}
	day := today()
	quotes, err := loadQuotes(ctx, day, keys)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not read quotes")
		return
	}

	waiting := 0
	views := make([]listView, 0, len(lists))
	for _, l := range lists {
		v := listView{
			ID:         l.ID.Hex(),
			Name:       l.Name,
			Items:      make([]itemView, 0, len(l.Items)),
			Pending:    orEmpty(l.Pending),
			Unresolved: orEmpty(l.Unresolved),
		}
		waiting += len(l.Pending)

		for _, it := range l.Items {
			iv := itemView{Symbol: it.Symbol, Name: it.Name, Exchange: it.Exchange, Type: it.Type, Spark: []float64{}}
			if q, ok := quotes[itemKey(it.Symbol, it.Exchange)]; ok {
				iv.Price, iv.Change, iv.ChangePct = q.Price, q.Change, q.ChangePct
				iv.Trend, iv.Error = q.Trend, q.Error
				if q.Spark != nil {
					iv.Spark = q.Spark
				}
			} else {
				iv.Loading = true
				waiting++
			}
			v.Items = append(v.Items, iv)
		}
		views = append(views, v)
	}

	c.JSON(http.StatusOK, gin.H{
		"date":          day,
		"keyConfigured": ct.client.HasKey(),
		"pending":       waiting,
		"lists":         views,
	})
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// CreateList makes a list; typed entries (comma separated symbols/names) are resolved in the background.
func (ct *Controller) CreateList(c *gin.Context) {
	var req struct {
		Name    string `json:"name"`
		Entries string `json:"entries"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request body")
		return
	}

	name, ok := cleanName(req.Name)
	if !ok {
		fail(c, http.StatusBadRequest, "name must be 1-60 characters")
		return
	}
	entries, err := parseEntries(req.Entries)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now()
	res, err := db.Lists().InsertOne(c.Request.Context(), models.List{
		Name:       name,
		Items:      []models.ListItem{},
		Pending:    orEmpty(entries),
		Unresolved: []string{},
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not create list")
		return
	}

	if len(entries) > 0 {
		ct.Kick()
	}
	id, _ := res.InsertedID.(bson.ObjectID)
	c.JSON(http.StatusCreated, gin.H{"id": id.Hex()})
}

// UpdateList renames a list and/or clears its "could not find" notes.
func (ct *Controller) UpdateList(c *gin.Context) {
	id, ok := objectID(c)
	if !ok {
		return
	}

	var req struct {
		Name            *string `json:"name"`
		ClearUnresolved bool    `json:"clearUnresolved"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request body")
		return
	}

	set := bson.M{"updatedAt": time.Now()}
	if req.Name != nil {
		name, ok := cleanName(*req.Name)
		if !ok {
			fail(c, http.StatusBadRequest, "name must be 1-60 characters")
			return
		}
		set["name"] = name
	}
	if req.ClearUnresolved {
		set["unresolved"] = []string{}
	}

	res, err := db.Lists().UpdateOne(c.Request.Context(), bson.M{"_id": id}, bson.M{"$set": set})
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not update list")
		return
	}
	if res.MatchedCount == 0 {
		fail(c, http.StatusNotFound, "list not found")
		return
	}
	c.Status(http.StatusNoContent)
}

func (ct *Controller) DeleteList(c *gin.Context) {
	id, ok := objectID(c)
	if !ok {
		return
	}

	res, err := db.Lists().DeleteOne(c.Request.Context(), bson.M{"_id": id})
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not delete list")
		return
	}
	if res.DeletedCount == 0 {
		fail(c, http.StatusNotFound, "list not found")
		return
	}
	c.Status(http.StatusNoContent)
}
