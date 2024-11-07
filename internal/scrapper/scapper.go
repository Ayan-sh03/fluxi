package scrapper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"scrapper/internal/db"
	"scrapper/internal/queue"
	"scrapper/pkg/logger"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Scraper struct {
	ctx            context.Context
	cancel         context.CancelFunc
	jobId          string
	visited        sync.Map
	links          []string
	linksMutex     sync.Mutex
	client         *http.Client
	concurrency    int
	rootDomain     string
	rootPath       string
	wg             sync.WaitGroup
	shutdown       chan struct{}
	isShuttingDown atomic.Bool
	stats          struct {
		processed  atomic.Int64
		errors     atomic.Int64
		maxLinks   int
		noMoreURLs bool // New field to track if we've exhausted all URLs
	}
	startTime     time.Time
	responseTimes []time.Duration
	queue         queue.QueueManager
	store         db.DB
}

func NewScraper(concurrency int, rootURL string, maxLinks int, jobId string) *Scraper {

	var q queue.QueueManager
	var err error
	parsedURL, _ := url.Parse(rootURL)
	ctx, cancel := context.WithCancel(context.Background())
	// Extract host and path
	host := parsedURL.Host
	path := parsedURL.Path

	// Try Redis first
	q, err = queue.NewRedisQueue("localhost:6379", rootURL)
	if err != nil {
		fmt.Printf("Redis unavailable, falling back to channel queue: %v", err)
		q = queue.NewChannelQueue(1000)
		fmt.Println("Using channel queue")
	} else {
		fmt.Println("Using Redis queue")
	}

	return &Scraper{
		ctx:    ctx,
		cancel: cancel,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		concurrency: concurrency,
		jobId:       jobId,
		rootDomain:  host,
		rootPath:    path,

		shutdown:       make(chan struct{}),
		isShuttingDown: atomic.Bool{},
		stats: struct {
			processed  atomic.Int64
			errors     atomic.Int64
			maxLinks   int
			noMoreURLs bool
		}{maxLinks: maxLinks},
		startTime: time.Now(),
		queue:     q,
	}

}

func (s *Scraper) restoreFromCheckpoint() error {
	fmt.Println("Restoring from checkpoint...")
	checkpointFile := "queue_checkpoint.json"

	data, err := os.ReadFile(checkpointFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No checkpoint exists
		}
		return err
	}

	var urls []string
	if err := json.Unmarshal(data, &urls); err != nil {
		return err
	}

	// Restore URLs to queue
	// for _, url := range urls {
	// 	s.urlQueue <- url
	// }

	logger.Logger.Printf("Restored %d URLs from checkpoint", len(urls))
	return nil
}

func (s *Scraper) saveQueueCheckpoint() {

	queueLength, err := s.queue.Len()
	if err != nil {
		logger.Logger.Printf("Error getting queue length: %v", err)
		return
	}
	if queueLength == 0 {
		return
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Create a slice to store current queue contents
			var queueContents []string
			// Iterate over the queue and add URLs to the slice
			//dequeu from the scrapper queue append it to queueContents and put it back

			for i := 0; i < queueLength; i++ {
				url, err := s.queue.Dequeue()
				if err != nil {
					logger.Logger.Printf("Error dequeuing URL: %v", err)
					continue
				}
				queueContents = append(queueContents, url)
				s.queue.Enqueue(url)
			}

			// Write to checkpoint file
			checkpointFile := "queue_checkpoint.json"
			data, err := json.Marshal(queueContents)
			if err != nil {
				logger.Logger.Printf("Error marshaling queue checkpoint: %v", err)
				continue
			}

			err = os.WriteFile(checkpointFile, data, 0644)
			if err != nil {
				logger.Logger.Printf("Error writing queue checkpoint: %v", err)
				continue
			}

			logger.Logger.Printf("Checkpoint saved: %d URLs in queue", len(queueContents))
		case <-s.shutdown:
			return
		}
	}
}

func (s *Scraper) tryIncrementProcessed() bool {
	newCount := s.stats.processed.Add(1)
	if newCount >= int64(s.stats.maxLinks) {
		s.cancel() // Cancel the context when maxLinks is reached
		return false
	}
	return true
}

func (s *Scraper) worker() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			logger.Logger.Printf("Context canceled, worker exiting.")
			return
		case <-s.shutdown:
			logger.Logger.Printf("Worker shutting down gracefully")
			s.updateJobStatus()
			s.queue.Clear()
			return
		default:
			// Proceed with processing URLs
			url, err := s.queue.Dequeue()
			if err != nil {
				logger.Logger.Printf("Error dequeuing URL: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if url == "" {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if !s.tryIncrementProcessed() {
				logger.Logger.Printf("Max links reached (%d). Context canceled.", s.stats.maxLinks)
				continue
			}

			logger.Logger.Printf("Processing URL: %s", url)
			markdown, err := s.HtmlToMarkdown(url)
			if err != nil {
				logger.Logger.Printf("Error processing %s: %v", url, err)
				s.incrementErrors()
				continue
			}
			s.WriteToFile(url, markdown)
			logger.Logger.Printf("Successfully processed URL: %s", url)
			err = s.updateScrappedUrls()
			if err != nil {
				logger.Logger.Printf("Error updating scrapped urls: %v", err)
			}
		}
	}
}
func (s *Scraper) Start(startURL string) {
	logger.Logger.Printf("Starting scraper with root URL: %s", startURL)

	// if err := s.restoreFromCheckpoint(); err != nil {
	// 	logger.Logger.Printf("Error restoring from checkpoint: %v", err)
	// }

	// // Start checkpoint saving goroutine
	// go s.saveQueueCheckpoint()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start workers
	for i := 0; i < s.concurrency; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	// go s.trackPerformance() // <-- Add this lin
	// Start initial URL
	// s.urlQueue <- startURL
	s.queue.Enqueue(startURL)

	// Wait for interrupt or completion
	go func() {
		<-sigChan
		logger.Logger.Println("Received shutdown signal. Gracefully shutting down...")
		close(s.shutdown)
		s.wg.Wait()
		os.Exit(0)
	}()

	// Wait for completion
	s.wg.Wait()
}

func (s *Scraper) addLink(link string) {
	s.linksMutex.Lock()
	s.links = append(s.links, link)
	s.linksMutex.Unlock()
}

func (s *Scraper) doRequestWithBackoff(req *http.Request) (*http.Response, error) {
	maxRetries := 6
	baseDelay := time.Second
	maxDelay := 64 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		res, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}

		// Check for rate limiting status codes (429 Too Many Requests)
		if res.StatusCode == 429 || (res.StatusCode >= 500 && res.StatusCode <= 599) {
			delay := time.Duration(1<<uint(attempt)) * baseDelay
			if delay > maxDelay {
				delay = maxDelay
			}

			logger.Logger.Printf("Rate limited (attempt %d/%d). Waiting %v before retry...",
				attempt+1, maxRetries, delay)

			res.Body.Close()
			time.Sleep(delay)
			continue
		}

		return res, nil
	}

	return nil, fmt.Errorf("max retries exceeded while handling rate limits")
}

func (s *Scraper) updateJobStatus() error {
	//update the job status status in the database
	jobStatus := "completed"
	// db := db.Db

	// defer db.Db.Close()
	err := db.Db.UpdateJobStatus(s.jobId, jobStatus)
	if err != nil {
		return err
	}
	return nil

}

func (s *Scraper) updateScrappedUrls() error {
	//update the scrapped urls in the database
	// fmt.Println("updating scrapped urls")

	s.linksMutex.Lock()
	scrappedUrls := make([]string, len(s.links))
	copy(scrappedUrls, s.links)
	s.linksMutex.Unlock()

	//marshal the scrapped urls to json
	scrappedUrlsJSON, err := json.Marshal(scrappedUrls)
	if err != nil {
		return err
	}

	db.Db.UpdateUrls(s.jobId, scrappedUrlsJSON)

	return nil
}
