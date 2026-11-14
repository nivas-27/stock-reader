package main

import (
	"context"
	"log"
	"os"
	"time"

	"backend/configs/db"
	"backend/configs/env"
	"backend/controllers/watchlists"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()
	env.CheckEnv()

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	// Watchlists (MongoDB + Twelve Data) are the whole app for now. The Kite /
	// Indian-stock code is unplugged here but still in the repo (controllers/kite*, utils, web/index.html).
	router.StaticFile("/", "./web/watchlists.html")
	router.StaticFile("/watchlists.js", "./web/watchlists.js")
	router.StaticFile("/watchlists.css", "./web/watchlists.css")

	dbCtx, cancelDB := context.WithTimeout(context.Background(), 10*time.Second)
	if err := db.Connect(dbCtx); err != nil {
		log.Println("MongoDB unavailable, watchlists disabled:", err)
	} else {
		watchlists.Register(router)
	}
	cancelDB()

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}
	log.Fatal(router.Run(addr)) // prints the reason if the port is already in use
}
