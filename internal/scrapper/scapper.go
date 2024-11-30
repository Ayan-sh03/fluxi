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
	"scrapper/internal/writer"
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
		noMoreURLs bool
	}
	startTime time.Time
	queue     queue.QueueManager
	store     db.DB
	writer    writer.Writer
	maxDepth  int
}

type UrlData struct {
	URL   string
	Depth int
}

func NewScraper(concurrency int, rootURL string, maxLinks int, jobId string, maxDepth int) *Scraper {

	var q queue.QueueManager
	var err error
	parsedURL, _ := url.Parse(rootURL)
	ctx, cancel := context.WithCancel(context.Background())
	// Extract host and path
	host := parsedURL.Host
	path := parsedURL.Path

	// !TODO: config should contain the writer and queue type, defaulting to file

	// Try Redis first
	q, err = queue.NewRedisQueue("localhost:6379", rootURL, maxLinks)
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
			Timeout: 120 * time.Second,
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
		writer:    writer.NewFileWriter("output"),
		maxDepth:  maxDepth,
	}

}

//temporary functions for enqueu and dequuqu for tesing depth based scrapper using redis

func (s *Scraper) enqueueUrl(url string, depth int) {
	urlData := UrlData{
		URL:   url,
		Depth: depth,
	}

	urlDataJSON, err := json.Marshal(urlData)
	if err != nil {
		logger.Logger.Printf("Error marshaling URL data: %v", err)
		return
	}
	s.queue.Enqueue(string(urlDataJSON))
}

func (s *Scraper) dequeueUrl() (*UrlData, error) {
	urlDataJSON, err := s.queue.Dequeue()
	if err != nil {
		return nil, err
	}
	var urlData UrlData
	if err := json.Unmarshal([]byte(urlDataJSON), &urlData); err != nil {
		return nil, err
	}
	return &urlData, nil
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
	for {
		current := s.stats.processed.Load()
		if current >= int64(s.stats.maxLinks) {
			return false
		}
		if s.stats.processed.CompareAndSwap(current, current+1) {
			if current+1 >= int64(s.stats.maxLinks) {
				logger.Logger.Printf("Max links reached: %d", s.stats.maxLinks)
				s.queue.Clear()
				s.cancel() // Cancel the context when maxLinks is reached
			}
			return true
		}
	}
}

func (s *Scraper) worker() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			logger.Logger.Printf("Finished job in %v seconds", time.Since(s.startTime).Seconds())

			logger.Logger.Printf("Context canceled, worker exiting.")
			s.updateJobStatus()
			s.queue.Clear()

			return
		case <-s.shutdown:
			logger.Logger.Printf("Finished job in %v seconds", time.Since(s.startTime).Seconds())

			logger.Logger.Printf("Worker shutting down gracefully")
			s.updateJobStatus()
			s.queue.Clear()
			return
		default:
			// Proceed with processing URLs
			urlDataJSON, err := s.queue.Dequeue()

			if err != nil {
				logger.Logger.Printf("Error dequeuing URL: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if urlDataJSON == "" {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			var urlData UrlData
			if err := json.Unmarshal([]byte(urlDataJSON), &urlData); err != nil {
				logger.Logger.Printf("Error unmarshaling URL data: %v", err)
				continue
			}

			if urlData.Depth >= s.maxDepth {
				logger.Logger.Printf("Max depth reached (%d). Context canceled.", s.maxDepth)
				if s.shouldComplete() {
					s.cancel()
				}
				continue
			}

			if !s.tryIncrementProcessed() {
				logger.Logger.Printf("Max links reached (%d). Context canceled.", s.stats.maxLinks)
				continue
			}

			logger.Logger.Printf("Processing URL: %s at depth: %d", urlData.URL, urlData.Depth)
			markdown, links, err := s.HtmlToText(urlData.URL)
			if err != nil {
				logger.Logger.Printf("Error processing %s: %v", urlData.URL, err)
				s.incrementErrors()
				continue
			}
			// fmt.Println("Found links:  ", links)

			for _, link := range links {
				logger.Logger.Printf("Processing link: %s", link)
				// Only mark as visited after successful processing
				newUrlData := UrlData{
					URL:   link,
					Depth: urlData.Depth + 1,
				}
				jsonData, err := json.Marshal(newUrlData)
				if err != nil {
					logger.Logger.Printf("Error marshaling URL data: %v", err)
					continue
				}

				logger.Logger.Printf("Enqueuing URL: %s at depth %d", link, urlData.Depth+1)
				if err := s.queue.Enqueue(string(jsonData)); err != nil {
					logger.Logger.Printf("Error enqueueing URL: %v", err)
				}
				s.addLink(link)
				s.visited.Store(link, true)
			}

			s.writer.Write(urlData.URL, markdown)
			logger.Logger.Printf("Successfully processed URL: %s at depth: %d", urlData.URL, urlData.Depth)
			err = s.updateScrappedUrls()
			if err != nil {
				logger.Logger.Printf("Error updating scrapped urls: %v", err)
			}
		}
	}
}

func (s *Scraper) shouldComplete() bool {
	queueLen, err := s.queue.Len()
	if err != nil {
		// Handle the error, maybe log it
		return false
	}
	return s.stats.processed.Load() >= int64(s.stats.maxLinks) || queueLen == 0
}
func (s *Scraper) Start(startURL string) {
	logger.Logger.Printf("Starting scraper with root URL: %s", startURL)
	s.startTime = time.Now()

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
	initialUrlData := UrlData{
		URL:   startURL,
		Depth: 0,
	}
	jsonData, err := json.Marshal(initialUrlData)
	if err != nil {
		logger.Logger.Printf("Error marshaling initial URL data: %v", err)
		return
	}
	s.queue.Enqueue(string(jsonData))

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
	fmt.Println("updating scrapped urls")

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
