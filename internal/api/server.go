package api

import (
	"log"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/gin-gonic/gin"
)

func Run() {
	router := gin.Default()

	//simple get route
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "Hello, World!"})
	})

	router.POST("/scrape", handleSingleURLScrape)
	router.POST("/scrape/full", handleFullScrape)
	router.GET("/scrape/status", GetJobStatus)
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	log.Println("Server started on port 8000")
	router.Run(":8000")

}
