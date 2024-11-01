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

func handleSingleURLScrape(c *gin.Context) {
	var req struct {
		URL string `json:"url" binding:"required"`
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

	scraper := scrapper.NewScraper(1, req.URL, 1, scrappedUrl.JobId)
	start := time.Now()
	markdown, err := scraper.HtmlToMarkdown(req.URL)

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

func handleFullScrape(c *gin.Context) {
	var req struct {
		RootURL     string `json:"rootUrl" binding:"required"`
		MaxURLs     int    `json:"maxUrls"`
		Concurrency int    `json:"concurrency"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "rootUrl is required in request body"})
		return
	}

	if req.MaxURLs == 0 {
		req.MaxURLs = 100
	}
	if req.Concurrency == 0 {
		req.Concurrency = 100
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

	scraper := scrapper.NewScraper(req.Concurrency, req.RootURL, req.MaxURLs, jobId)

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
