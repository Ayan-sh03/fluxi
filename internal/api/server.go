package api

import (
	"log"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/gin-gonic/gin"
)

func Run() {
	router := gin.Default()

	router.POST("/scrape", handleSingleURLScrape)
	router.POST("/scrape/full", handleFullScrape)
	router.GET("/scrape/status", GetJobStatus)
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	log.Println("Server started on port 8080")
	router.Run(":8080")

}
