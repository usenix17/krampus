package main

import (
	"context"
	"embed"
	"io/fs"
	"krampus/server/config"
	"krampus/server/database"
	"krampus/server/handlers"
	"krampus/server/middleware"
	"krampus/server/services"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed static
var staticFiles embed.FS

func main() {
	// Load configuration
	config.Load()

	// Initialize database
	if err := database.Initialize(config.AppConfig.DatabasePath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Initialize OIDC provider
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := services.InitializeOIDC(ctx); err != nil {
		log.Printf("WARNING: OIDC initialization failed: %v", err)
		log.Println("OIDC authentication will not be available")
	} else {
		log.Println("OIDC provider initialized successfully")
	}

	// Setup Gin router
	router := gin.Default()

	// Apply CORS middleware
	router.Use(middleware.CORSMiddleware())

	// Serve embedded static files
	// Create filesystem for assets directory
	assetsFS, err := fs.Sub(staticFiles, "static/assets")
	if err != nil {
		log.Printf("Warning: Failed to load assets: %v", err)
	} else {
		router.StaticFS("/assets", http.FS(assetsFS))
		log.Println("Embedded frontend assets loaded successfully")
	}

	// Serve vite.svg
	router.GET("/vite.svg", func(c *gin.Context) {
		data, err := staticFiles.ReadFile("static/vite.svg")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "image/svg+xml", data)
	})

	// Serve krampus.svg
	router.GET("/krampus.svg", func(c *gin.Context) {
		data, err := staticFiles.ReadFile("static/krampus.svg")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "image/svg+xml", data)
	})

	// Serve index.html at root
	router.GET("/", func(c *gin.Context) {
		data, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Page not found"})
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	// Health check endpoint (public)
	router.GET("/ping", handlers.Health)

	// Public config endpoint
	router.GET("/api/config", handlers.GetPublicConfig)

	// Authentication routes (public)
	authGroup := router.Group("/auth")
	{
		authGroup.GET("/login", handlers.Login)
		authGroup.GET("/callback", handlers.Callback)
		authGroup.POST("/logout", handlers.Logout)
		authGroup.GET("/me", middleware.AuthMiddleware(), handlers.Me)
	}

	// API routes (protected)
	api := router.Group("/api")
	api.Use(middleware.AuthMiddleware())
	{
		// Proposals
		proposalsGroup := api.Group("/proposals")
		{
			proposalsGroup.GET("", handlers.ListProposals)
			proposalsGroup.GET("/:id", handlers.GetProposal)
			proposalsGroup.POST("", handlers.CreateProposal)
			proposalsGroup.POST("/:id/vote", handlers.VoteOnProposal)
			proposalsGroup.DELETE("/:id", handlers.DeleteProposal)

			// Admin-only proposal routes
			proposalsGroup.POST("/:id/approve", middleware.AdminMiddleware(), handlers.ApproveProposal)
		}

		// Rules
		rulesGroup := api.Group("/rules")
		{
			rulesGroup.GET("", handlers.ListRules)
			rulesGroup.GET("/:id", handlers.GetRule)

			// Admin-only rule routes
			rulesGroup.POST("", middleware.AdminMiddleware(), handlers.CreateRule)
			rulesGroup.DELETE("/:id", middleware.AdminMiddleware(), handlers.DeleteRule)
		}

		// Machines
		machinesGroup := api.Group("/machines")
		{
			machinesGroup.GET("", handlers.ListMachines)
			machinesGroup.GET("/:id", handlers.GetMachine)
			machinesGroup.POST("", handlers.RegisterMachine)
			machinesGroup.POST("/:id/mobileconfig", handlers.GenerateMobileConfig)

			// Admin-only machine routes
			machinesGroup.DELETE("/:id", middleware.AdminMiddleware(), handlers.DeleteMachine)
		}

		// Events
		eventsGroup := api.Group("/events")
		{
			eventsGroup.GET("", handlers.ListEvents)
		}

		// Programs
		programsGroup := api.Group("/programs")
		{
			programsGroup.GET("", handlers.ListPrograms)
		}

		// Users (admin-only)
		usersGroup := api.Group("/users")
		usersGroup.Use(middleware.AdminMiddleware())
		{
			usersGroup.GET("", handlers.ListUsers)
			usersGroup.GET("/:id", handlers.GetUser)
			usersGroup.PUT("/:id", handlers.UpdateUser)
			usersGroup.DELETE("/:id", handlers.DeleteUser)
		}
	}

	// Santa sync protocol endpoints (machine authentication would go here)
	santaGroup := router.Group("", middleware.Decompress())
	{
		santaGroup.POST("/preflight/:machine_id", handlers.Preflight)
		santaGroup.POST("/eventupload/:machine_id", handlers.EventUpload)
		santaGroup.POST("/ruledownload/:machine_id", handlers.RuleDownload)
		santaGroup.POST("/postflight/:machine_id", handlers.Postflight)
	}

	// Serve index.html for all other routes (SPA routing)
	router.NoRoute(func(c *gin.Context) {
		data, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Page not found"})
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	// Periodic cleanup of expired sessions
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := services.CleanupExpiredSessions(); err != nil {
				log.Printf("Failed to cleanup expired sessions: %v", err)
			}
		}
	}()

	// Start server
	serverAddr := ":" + config.AppConfig.ServerPort
	log.Printf("Starting Krampus Santa Sync Server on %s", serverAddr)
	log.Printf("Sync Base URL: %s", config.AppConfig.SyncBaseURL)
	log.Printf("Vote Threshold: %d", config.AppConfig.VoteThreshold)

	if err := router.Run(serverAddr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
