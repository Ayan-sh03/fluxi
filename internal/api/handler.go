// Package api provides the HTTP handlers for the scraper service
// @title Web Scraper API
// @version 1.0
// @description Service for scraping websites and converting them to markdown
// @host localhost:8080
// @BasePath /
package api

import (
	"fmt"
	"log"
	"net/url"
	"scrapper/internal/db"
	"scrapper/internal/models"
	"scrapper/internal/scrapper"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// swagger:route POST /scrape/single scraping singleScrape
// Scrapes a single URL and converts to markdown.
// responses:
//
//	200: singleScrapeResponse
//	400: errorResponse
//	500: errorResponse
func handleSingleURLScrape(c *gin.Context) {
	var req struct {
		URL        string `json:"url" binding:"required"`
		WebhookURL string `json:"webhookUrl"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "url is required in request body"})
		return
	}

	var scrappedUrl models.ScrappedUrl
	scrappedUrl.JobId = uuid.New().String()
	scrappedUrl.ScrappedUrls = []string{req.URL}

	err := db.Db.Insert(scrappedUrl)
	if err != nil {
		log.Fatal(err)
	}

	scraper := scrapper.NewScraper(1, req.URL, 1, scrappedUrl.JobId, 0, req.WebhookURL)
	start := time.Now()
	markdown, _, err := scraper.HtmlToText(req.URL)

	log.Printf("Time taken for HTML to Markdown conversion: %v", time.Since(start))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("output/%s.md", url.QueryEscape(req.URL))
	go scraper.WriteToFile(req.URL, markdown)

	c.JSON(200, gin.H{
		"status":      "success",
		"url":         req.URL,
		"output_file": filename,
	})
}

// swagger:route POST /scrape/full scraping fullScrape
// Performs full recursive website scraping.
// responses:
//
//	200: fullScrapeResponse
//	400: errorResponse
func handleFullScrape(c *gin.Context) {
	var req struct {
		RootURL     string `json:"rootUrl" binding:"required"`
		MaxURLs     int    `json:"maxUrls"`
		Concurrency int    `json:"concurrency"`
		MaxDepth    int    `json:"maxDepth"`
		WebhookURL  string `json:"webhookUrl"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "rootUrl is required in request body"})
		return
	}

	if req.WebhookURL == "" {
		c.JSON(400, gin.H{"error": "webhookUrl is required in request body"})
		return
	}

	if req.MaxURLs == 0 {
		req.MaxURLs = 100
	}
	if req.Concurrency == 0 {
		req.Concurrency = 100
	}

	if req.MaxDepth == 0 {
		req.MaxDepth = 3
	}

	jobId := uuid.New().String()
	scrappedUrl := models.ScrappedUrl{
		JobId:        jobId,
		ScrappedUrls: []string{req.RootURL},
		Status:       "pending",
	}
	err := db.Db.Insert(scrappedUrl)
	if err != nil {
		log.Fatal(err)
	}

	scraper := scrapper.NewScraper(req.Concurrency, req.RootURL, req.MaxURLs, jobId, req.MaxDepth, req.WebhookURL)

	go func() {
		scraper.Start(req.RootURL)
	}()

	c.JSON(200, gin.H{
		"status":      "started",
		"job_id":      jobId,
		"root_url":    req.RootURL,
		"max_urls":    req.MaxURLs,
		"concurrency": req.Concurrency,
	})
}

// swagger:route GET /job/status jobs getStatus
// Get job status by ID.
// responses:
//
//	200: jobStatusResponse
//	400: errorResponse
//	404: errorResponse
func GetJobStatus(c *gin.Context) {
	jobId := c.Query("jobId")
	if jobId == "" {
		c.JSON(400, gin.H{"error": "jobId is required"})
		return
	}

	scrappedUrl, err := db.Db.Get(jobId)
	// fmt.Print("from db: ", scrappedUrl)
	if err != nil {
		c.JSON(404, gin.H{"error": "Job not found"})
		log.Println(err)
		return
	}
	c.JSON(200, scrappedUrl)

}
