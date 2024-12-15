package api

import (
	"fmt"
	"log"
	"net/http"

	swaggerFiles "github.com/swaggo/files"

	_ "scrapper/docs"
	"scrapper/internal/api"
	"scrapper/internal/middleware"

	"github.com/gin-gonic/gin"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func Run() {
	router := gin.Default()
	router.LoadHTMLGlob("templates/*")
	//simple get route
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "Hello, World!"})
	})

	router.POST("/scrape", api.HandleSingleURLScrape)
	router.POST("/scrape/full", api.HandleFullScrape)
	router.GET("/scrape/status", api.GetJobStatus)
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	router.GET("/auth", func(c *gin.Context) {
		c.HTML(http.StatusOK, "auth.html", nil)
	})

	router.GET("/api-keys", func(c *gin.Context) {
		c.HTML(http.StatusOK, "api-keys.html", nil)
	})

	router.POST("/api/signup", api.HandleSignup)
	router.POST("/api/login", api.HandleLogin)

	protected := router.Group("/api")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.GET("/", func(c *gin.Context) {
			userID, exists := c.Get("user_id")
			if !exists {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - No user ID provided"})
				return
			}
			fmt.Println("protected.GET::", userID)
			c.JSON(200, gin.H{"message": "Hello, World!"})
		})
		protected.GET("/keys", api.HandleGetApiKeys)
		protected.POST("/generate-api-key",
			api.HandleGenerateAPIKey)

		keyProtected := router.Group("/auth/")
		keyProtected.Use(middleware.APIKeyMiddleware())
		{
			keyProtected.GET("/key", func(c *gin.Context) {
				apiKey := c.GetHeader("X-API-KEY")
				if apiKey == "" {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - No API key provided"})
					return
				}
				fmt.Println("keyProtected.GET::", apiKey)

				c.JSON(200, gin.H{"message": "Hello, World!"})
			})
		}
	}

	log.Println("Server started on port 8000")
	router.Run(":8000")

}
