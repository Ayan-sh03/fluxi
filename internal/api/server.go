package api

import (
	"log"
	"net/http"

	swaggerFiles "github.com/swaggo/files"

	_ "scrapper/docs"

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

	router.POST("/scrape", handleSingleURLScrape)
	router.POST("/scrape/full", handleFullScrape)
	router.GET("/scrape/status", GetJobStatus)
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	router.GET("/auth", func(c *gin.Context) {
		c.HTML(http.StatusOK, "auth.html", nil)
	})

	router.GET("/api-keys", func(c *gin.Context) {
		c.HTML(http.StatusOK, "api-keys.html", nil)
	})

	log.Println("Server started on port 8000")
	router.Run(":8000")

}
