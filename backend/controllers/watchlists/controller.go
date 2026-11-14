package watchlists

import (
	"context"
	"log"
	"strings"
	"time"

	"backend/utils/twelvedata"

	"github.com/gin-gonic/gin"
)

const (
	maxListNameLen  = 60
	maxEntryLen     = 80
	maxEntriesBatch = 50
	maxItemsPerList = 35
)

// Controller owns the Twelve Data client and wakes the background worker.
type Controller struct {
	client *twelvedata.Client
	kick   chan struct{}
}

// Register mounts the /api routes and starts the background worker.
// MongoDB must already be connected.
func Register(r *gin.Engine) {
	ct := &Controller{client: twelvedata.New(), kick: make(chan struct{}, 1)}
	if !ct.client.HasKey() {
		log.Println("watchlists:", twelvedata.ErrNoKey, "- lists work, but search and prices stay empty until it is set")
	}

	r.GET("/api/lists", ct.GetLists)
	r.POST("/api/lists", ct.CreateList)
	r.PATCH("/api/lists/:id", ct.UpdateList)
	r.DELETE("/api/lists/:id", ct.DeleteList)

	r.POST("/api/lists/:id/items", ct.AddItem)
	r.DELETE("/api/lists/:id/items", ct.RemoveItem)
	r.POST("/api/lists/:id/entries", ct.AddEntries)

	r.GET("/api/search", ct.Search)

	seedCtx, cancelSeed := context.WithTimeout(context.Background(), 10*time.Second)
	ensureSeedLists(seedCtx)
	cancelSeed()

	go ct.runWorker()
}

// Kick asks the worker to run a pass soon; it never blocks.
func (ct *Controller) Kick() {
	select {
	case ct.kick <- struct{}{}:
	default:
	}
}

func (ct *Controller) kickAfter(d time.Duration) { time.AfterFunc(d, ct.Kick) }

func today() string { return time.Now().Format("2006-01-02") }

func itemKey(symbol, exchange string) string { return symbol + "|" + exchange }

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": msg})
}

func cleanName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	n := len([]rune(name))
	return name, n >= 1 && n <= maxListNameLen
}
