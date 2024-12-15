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

// SingleScrapeRequest represents the request body for single URL scraping
type SingleScrapeRequest struct {
	URL        string `json:"url" binding:"required"`
	WebhookURL string `json:"webhookUrl"`
}

// ScrapeResponse represents the response for a scrape request
type ScrapeResponse struct {
	Status     string `json:"status"`
	URL        string `json:"url"`
	OutputFile string `json:"output_file"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

type FullScrapeResponse struct {
	Status      string `json:"status"`
	JobId       string `json:"job_id"`
	RootURL     string `json:"root_url"`
	MaxURLs     int    `json:"max_urls"`
	Concurrency int    `json:"concurrency"`
}

// FullScrapeRequest represents the request body for full website scraping
type FullScrapeRequest struct {
	RootURL     string `json:"rootUrl" binding:"required"`
	MaxURLs     int    `json:"maxUrls"`
	Concurrency int    `json:"concurrency"`
	MaxDepth    int    `json:"maxDepth"`
	WebhookURL  string `json:"webhookUrl"`
}

// @Summary Scrape single URL
// @Description Scrapes a single URL and converts its content to markdown
// @Tags scraping
// @Accept json
// @Produce json
// @Param request body SingleScrapeRequest true "Scrape request parameters"
// @Success 200 {object} ScrapeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /scrape [post]
// @Security ApiKeyAuth
func HandleSingleURLScrape(c *gin.Context) {
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

// @Summary Scrape full website
// @Description Scrapes a website recursively starting from a root URL
// @Tags scraping
// @Accept json
// @Produce json
// @Param request body api.FullScrapeRequest true "Full scrape request parameters"
// @Success 200 {object} FullScrapeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /scrape/full [post]
// @Security ApiKeyAuth
func HandleFullScrape(c *gin.Context) {
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

// @Summary Get job status
// @Description Retrieves the current status of a scraping job
// @Tags Jobs
// @Accept json
// @Produce json
// @Param jobId query string true "Job ID" Format(uuid)
// @Success 200 {object} models.ScrappedUrl
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /scrape/status [get]
// @Security ApiKeyAuth
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

// @Summary Generate API key
// @Description Generates a new API key for the user
// @Tags API Keys
// @Accept json
// @Produce json
// @Success 200 {object} models.APIKey
// @Failure 500 {object} ErrorResponse
// @Router /api-key [post]
// @Security ApiKeyAuth
func HandleGenerateAPIKey(c *gin.Context) {
	apiKey := models.NewAPIKey(uuid.MustParse(c.GetString("user_id")), "test")
	err := db.Db.CreateAPIKey(apiKey)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to generate API key"})
		return
	}
	c.JSON(200, apiKey)
}
