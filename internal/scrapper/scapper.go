package scrapper

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"scrapper/internal/queue"
	"scrapper/pkg/logger"
	"sync"
	"syscall"
	"time"
)

type Scraper struct {
	visited     sync.Map
	links       []string
	linksMutex  sync.Mutex
	client      *http.Client
	concurrency int
	rootDomain  string
	urlQueue    chan string
	wg          sync.WaitGroup
	shutdown    chan struct{}
	stats       struct {
		processed  int
		errors     int
		maxLinks   int
		noMoreURLs bool // New field to track if we've exhausted all URLs
		sync.Mutex
	}
	startTime     time.Time
	responseTimes []time.Duration
	queue         queue.QueueManager
}

func NewScraper(concurrency int, rootURL string, maxLinks int) *Scraper {

	q, err := queue.NewRedisQueue("localhost:6379")
	if err != nil {
		log.Fatalf("Failed to create Redis queue: %v", err)
	}

	return &Scraper{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		concurrency: concurrency,
		rootDomain:  extractDomain(rootURL),
		urlQueue:    make(chan string, 1000),
		shutdown:    make(chan struct{}),
		stats: struct {
			processed  int
			errors     int
			maxLinks   int
			noMoreURLs bool
			sync.Mutex
		}{maxLinks: maxLinks},
		startTime: time.Now(),
		queue:     q,
	}
}

func (s *Scraper) Start(startURL string) {
	logger.Printf("Starting scraper with root URL: %s", startURL)

	if err := s.restoreFromCheckpoint(); err != nil {
		logger.Printf("Error restoring from checkpoint: %v", err)
	}

	// Start checkpoint saving goroutine
	go s.saveQueueCheckpoint()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start workers
	for i := 0; i < s.concurrency; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	go s.trackPerformance() // <-- Add this lin
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
