package api

import (
	"log"

	"github.com/gin-gonic/gin"
)

func Run() {
	router := gin.Default()

	router.POST("/scrape", handleSingleURLScrape)
	router.POST("/scrape/full", handleFullScrape)
	router.GET("/scrape/status", GetJobStatus)

	log.Println("Server started on port 8080")
	router.Run(":8080")

}
