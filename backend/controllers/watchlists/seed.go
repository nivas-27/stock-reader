package watchlists

import (
	"context"
	"log"
	"time"

	"backend/configs/db"
	"backend/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// seedList is one curated list shipped with the app. Visitors cannot create
// lists from the page, so new ones are added here and picked up on restart.
type seedList struct {
	Name  string
	Items []models.ListItem
}

func stock(symbol, name, exchange string) models.ListItem {
	return models.ListItem{Symbol: symbol, Name: name, Exchange: exchange, Type: "Common Stock"}
}

// seedLists holds every list the site offers. Keep each one at or under maxItemsPerList.
var seedLists = []seedList{
	{
		Name: "AI Leaders",
		Items: []models.ListItem{
			stock("NVDA", "NVIDIA Corporation", "NASDAQ"),
			stock("AMD", "Advanced Micro Devices, Inc.", "NASDAQ"),
			stock("AVGO", "Broadcom Inc.", "NASDAQ"),
			stock("TSM", "Taiwan Semiconductor Manufacturing Company Limited", "NYSE"),
			stock("MU", "Micron Technology, Inc.", "NASDAQ"),
			stock("ARM", "Arm Holdings plc", "NASDAQ"),
			stock("MSFT", "Microsoft Corporation", "NASDAQ"),
			stock("GOOGL", "Alphabet Inc.", "NASDAQ"),
			stock("META", "Meta Platforms, Inc.", "NASDAQ"),
			stock("AMZN", "Amazon.com, Inc.", "NASDAQ"),
			stock("AAPL", "Apple Inc.", "NASDAQ"),
			stock("PLTR", "Palantir Technologies Inc.", "NASDAQ"),
			stock("SNOW", "Snowflake Inc.", "NYSE"),
			stock("CRM", "Salesforce, Inc.", "NYSE"),
			stock("NOW", "ServiceNow, Inc.", "NYSE"),
			stock("ORCL", "Oracle Corporation", "NYSE"),
			stock("SMCI", "Super Micro Computer, Inc.", "NASDAQ"),
			stock("DELL", "Dell Technologies Inc.", "NYSE"),
			stock("ANET", "Arista Networks, Inc.", "NYSE"),
			stock("VRT", "Vertiv Holdings Co", "NYSE"),
		},
	},
}

// ensureSeedLists inserts any curated list that is not in the database yet.
// Existing lists are left alone, so edits made in the app survive a restart.
func ensureSeedLists(ctx context.Context) {
	for _, s := range seedLists {
		n, err := db.Lists().CountDocuments(ctx, bson.M{"name": s.Name})
		if err != nil {
			log.Println("watchlists: could not check seed list", s.Name, err)
			continue
		}
		if n > 0 {
			continue
		}

		items := s.Items
		if len(items) > maxItemsPerList {
			items = items[:maxItemsPerList]
		}
		now := time.Now()
		if _, err := db.Lists().InsertOne(ctx, models.List{
			Name:       s.Name,
			Items:      items,
			Pending:    []string{},
			Unresolved: []string{},
			CreatedAt:  now,
			UpdatedAt:  now,
		}); err != nil {
			log.Println("watchlists: could not add seed list", s.Name, err)
			continue
		}
		log.Printf("watchlists: added seed list %q with %d symbols", s.Name, len(items))
	}
}
