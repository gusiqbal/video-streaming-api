package main

import (
	"log"
	"mime"
	"os"

	"gosocket/internal/config"
	"gosocket/internal/database"
	"gosocket/internal/handlers"
	"gosocket/internal/hub"
	"gosocket/internal/repository"
	"gosocket/internal/services"

	"github.com/gin-gonic/gin"
)

func main() {
	// Register MIME types for HLS files so http.ServeFile sets the correct Content-Type.
	mime.AddExtensionType(".m3u8", "application/vnd.apple.mpegurl")
	mime.AddExtensionType(".ts", "video/mp2t")

	cfg := config.Load()

	if err := os.MkdirAll(cfg.StoragePath, 0755); err != nil {
		log.Fatalf("create storage dir: %v", err)
	}

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	h := hub.New()
	go h.Run()

	repo := repository.NewVideoRepo(db)
	transcoder := services.NewTranscoder(cfg.FFmpegPath, cfg.FFprobePath, repo, h)
	videoHandler := handlers.NewVideoHandler(repo, transcoder, h, cfg.StoragePath)
	wsHandler := handlers.NewWSHandler(h)

	r := gin.Default()
	r.Use(corsMiddleware())
	// Catch-all OPTIONS so browser preflight requests always hit the CORS middleware.
	r.OPTIONS("/*path", func(c *gin.Context) {})

	api := r.Group("/api/videos")
	{
		api.POST("", videoHandler.Create)
		api.PUT("/:id/upload", videoHandler.Upload)
		api.GET("/:id", videoHandler.Get)
	}

	// HLS file serving — path mirrors filesystem so relative URLs in playlists resolve correctly.
	// e.g. GET /hls/<id>/master.m3u8
	//      GET /hls/<id>/720p/index.m3u8
	//      GET /hls/<id>/720p/000.ts
	r.GET("/hls/:id/*filepath", videoHandler.ServeHLS)

	r.GET("/ws", wsHandler.Handle)

	log.Printf("listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
