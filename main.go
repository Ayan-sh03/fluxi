package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// Initialize logger
var logger *log.Logger

type PerformanceMetrics struct {
	startCPUTime    float64
	cpuUtilization  float64
	memoryUsed      uint64
	goroutineCount  int
	requestsPerSec  float64
	totalRequests   int
	successRate     float64
	avgResponseTime time.Duration
	peakMemoryUsage uint64
	mutex           sync.Mutex
}

func init() {
	logFile, err := os.OpenFile("scraperwithCPU.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	logger = log.New(logFile, "", log.LstdFlags)
	// Also log to stdout
	log.SetOutput(os.Stdout)
}

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
	performance   PerformanceMetrics
	responseTimes []time.Duration
	queue         QueueManager
}

type QueueManager interface {
	Enqueue(url string) error
	Dequeue() (string, error)
}
type RedisQueue struct {
	client *redis.Client
	key    string
	ctx    context.Context
}
type QueueItem struct {
	URL       string    `json:"url"`
	Timestamp time.Time `json:"timestamp"`
	Retries   int       `json:"retries"`
	Priority  int       `json:"priority"`
}

func NewRedisQueue(addr string) (*RedisQueue, error) {
	client := redis.NewClient(&redis.Options{
		Addr:            addr,
		MaxRetries:      5,
		MaxRetryBackoff: time.Second * 3,
		MinRetryBackoff: time.Millisecond * 100,
		ReadTimeout:     time.Second * 3,
		WriteTimeout:    time.Second * 3,
		PoolSize:        10,
		MinIdleConns:    5,
	})

	ctx := context.Background()
	if err := client.Ping().Err(); err != nil {
		return nil, err
	}

	return &RedisQueue{
		client: client,
		key:    "scraper:urls",
		ctx:    ctx,
	}, nil
}

func (q *RedisQueue) Enqueue(url string) error {
	item := QueueItem{
		URL:       url,
		Timestamp: time.Now(),
		Retries:   0,
		Priority:  1,
	}

	data, err := json.Marshal(item)
	if err != nil {
		return err
	}

	return q.client.LPush(q.key, data).Err()
}

func (q *RedisQueue) Dequeue() (string, error) {
	result, err := q.client.RPop(q.key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	var item QueueItem
	if err := json.Unmarshal([]byte(result), &item); err != nil {
		return "", err
	}

	return item.URL, nil
}

func (s *Scraper) saveQueueCheckpoint() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Create a slice to store current queue contents
			var queueContents []string

			// Safely get queue length
			queueLen := len(s.urlQueue)

			// Read from queue without removing items
			for i := 0; i < queueLen; i++ {
				select {
				case url := <-s.urlQueue:
					queueContents = append(queueContents, url)
					// Put it back in the queue
					s.urlQueue <- url
				default:
					continue
				}
			}

			// Write to checkpoint file
			checkpointFile := "queue_checkpoint.json"
			data, err := json.Marshal(queueContents)
			if err != nil {
				logger.Printf("Error marshaling queue checkpoint: %v", err)
				continue
			}

			err = os.WriteFile(checkpointFile, data, 0644)
			if err != nil {
				logger.Printf("Error writing queue checkpoint: %v", err)
				continue
			}

			logger.Printf("Checkpoint saved: %d URLs in queue", len(queueContents))
		case <-s.shutdown:
			return
		}
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
	for _, url := range urls {
		s.urlQueue <- url
	}

	logger.Printf("Restored %d URLs from checkpoint", len(urls))
	return nil
}

func (s *Scraper) trackPerformance() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.updatePerformanceMetrics()
		case <-s.shutdown:
			return
		}
	}
}

func (s *Scraper) updatePerformanceMetrics() {
	s.performance.mutex.Lock()
	defer s.performance.mutex.Unlock()

	// Update CPU usage
	cpuPercent, err := cpu.Percent(0, false)
	if err == nil && len(cpuPercent) > 0 {
		s.performance.cpuUtilization = cpuPercent[0]
	}

	// Update memory usage
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	s.performance.memoryUsed = mem.Alloc
	if mem.Alloc > s.performance.peakMemoryUsage {
		s.performance.peakMemoryUsage = mem.Alloc
	}

	// Update goroutine count
	s.performance.goroutineCount = runtime.NumGoroutine()

	// Calculate requests per second
	duration := time.Since(s.startTime).Seconds()
	if duration > 0 {
		s.performance.requestsPerSec = float64(s.stats.processed) / duration
	}

	// Calculate success rate
	totalRequests := s.stats.processed + s.stats.errors
	if totalRequests > 0 {
		s.performance.successRate = float64(s.stats.processed) / float64(totalRequests) * 100
	}

	// Calculate average response time
	if len(s.responseTimes) > 0 {
		var total time.Duration
		for _, t := range s.responseTimes {
			total += t
		}
		s.performance.avgResponseTime = total / time.Duration(len(s.responseTimes))
	}
}

func NewScraper(concurrency int, rootURL string, maxLinks int) *Scraper {

	q, err := NewRedisQueue("localhost:6379")
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

func extractDomain(urlString string) string {
	u, err := url.Parse(urlString)
	if err != nil {
		return ""
	}
	return u.Host
}

func (s *Scraper) isValidURL(urlString string) bool {
	if urlString == "" || strings.HasPrefix(urlString, "#") {
		return false
	}
	u, err := url.Parse(urlString)
	if err != nil {
		return false
	}
	return u.Host == s.rootDomain || u.Host == ""
}

// func (s *Scraper) incrementProcessed() {
// 	s.stats.Lock()
// 	s.stats.processed++
// 	if s.stats.processed >= s.stats.maxLinks {
// 		logger.Printf("Reached maximum link count of %d, initiating shutdown...", s.stats.maxLinks)
// 		close(s.shutdown)
// 	}
// 	s.stats.Unlock()
// }

func (s *Scraper) addLink(link string) {
	s.linksMutex.Lock()
	s.links = append(s.links, link)
	s.linksMutex.Unlock()
}
func (s *Scraper) printPerformanceStats() {
	v, _ := mem.VirtualMemory()
	cpuCount := runtime.NumCPU()

	logger.Printf("\nPerformance Metrics:")
	logger.Printf("------------------------")
	logger.Printf("CPU Metrics:")
	logger.Printf("  - CPU Utilization: %.2f%%", s.performance.cpuUtilization)
	logger.Printf("  - Available CPU Cores: %d", cpuCount)
	logger.Printf("  - Goroutines Used: %d", s.performance.goroutineCount)

	logger.Printf("\nMemory Metrics:")
	logger.Printf("  - Current Memory Usage: %s", formatBytes(s.performance.memoryUsed))
	logger.Printf("  - Peak Memory Usage: %s", formatBytes(s.performance.peakMemoryUsage))
	logger.Printf("  - System Memory Available: %s", formatBytes(v.Available))
	logger.Printf("  - Memory Usage %%: %.2f%%", float64(s.performance.memoryUsed)/float64(v.Total)*100)

	logger.Printf("\nPerformance Indicators:")
	logger.Printf("  - Average Processing Speed: %.2f URLs/second", s.performance.requestsPerSec)
	logger.Printf("  - Success Rate: %.2f%%", s.performance.successRate)
	logger.Printf("  - Average Response Time: %v", s.performance.avgResponseTime)

	logger.Printf("\nEfficiency Metrics:")
	logger.Printf("  - URLs per Goroutine: %.2f", float64(s.stats.processed)/float64(s.performance.goroutineCount))
	logger.Printf("  - Memory per URL: %s", formatBytes(s.performance.memoryUsed/uint64(s.stats.processed)))
	logger.Printf("  - CPU Efficiency Score: %.2f", calculateCPUEfficiency(s.performance.cpuUtilization, s.performance.requestsPerSec))
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func calculateCPUEfficiency(cpuUsage, requestsPerSec float64) float64 {
	if cpuUsage == 0 {
		return 0
	}
	// Higher score is better - more requests per CPU percentage
	return (requestsPerSec * 100) / cpuUsage
}

func (s *Scraper) htmlToMarkdown(url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	startTime := time.Now()
	defer func() {
		s.responseTimes = append(s.responseTimes, time.Since(startTime))
	}()

	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MyScraper/1.0)")

	res, err := s.doRequestWithBackoff(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return "", fmt.Errorf("failed to fetch URL: %s (status: %d)", url, res.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", err
	}

	var markdown strings.Builder
	var wg sync.WaitGroup

	// Process different HTML elements concurrently
	wg.Add(4)

	// Process headings
	go func() {
		defer wg.Done()
		s.processHeadings(doc, &markdown)
	}()

	// Process paragraphs and text
	go func() {
		defer wg.Done()
		s.processText(doc, &markdown)
	}()

	// Process links
	go func() {
		defer wg.Done()
		s.processLinks(doc, url)
	}()

	// Process other elements
	go func() {
		defer wg.Done()
		s.processOtherElements(doc, &markdown)
	}()

	wg.Wait()

	return strings.TrimSpace(markdown.String()), nil
}

func (s *Scraper) processHeadings(doc *goquery.Document, markdown *strings.Builder) {
	var mu sync.Mutex
	doc.Find("h1, h2, h3, h4, h5, h6").Each(func(i int, sel *goquery.Selection) {
		tag := goquery.NodeName(sel)
		headingText := strings.TrimSpace(sel.Text())
		if headingText != "" {
			mu.Lock()
			markdown.WriteString(strings.Repeat("#", int(tag[1]-'0')) + " " + headingText + "\n\n")
			mu.Unlock()
		}
	})
}

func (s *Scraper) processText(doc *goquery.Document, markdown *strings.Builder) {
	var mu sync.Mutex
	doc.Find("p").Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())
		if text != "" {
			mu.Lock()
			markdown.WriteString(text + "\n\n")
			mu.Unlock()
		}
	})
}

func (s *Scraper) processLinks(doc *goquery.Document, baseURL string) {
	doc.Find("a").Each(func(i int, sel *goquery.Selection) {
		href, exists := sel.Attr("href")
		if !exists {
			return
		}

		absoluteURL := s.makeAbsoluteURL(href, baseURL)
		if s.isValidURL(absoluteURL) {
			if _, loaded := s.visited.LoadOrStore(absoluteURL, true); !loaded {
				s.addLink(absoluteURL)
				// Replace channel send with Redis enqueue
				err := s.queue.Enqueue(absoluteURL)
				if err != nil {
					logger.Printf("Error enqueueing URL: %v", err)
				}
			}
		}
	})
}

func (s *Scraper) makeAbsoluteURL(href, baseURL string) string {
	relativeURL, err := url.Parse(href)
	if err != nil {
		return ""
	}

	baseURLParsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	absoluteURL := baseURLParsed.ResolveReference(relativeURL)
	return absoluteURL.String()
}

func (s *Scraper) processOtherElements(doc *goquery.Document, markdown *strings.Builder) {
	var mu sync.Mutex

	// Process blockquotes
	doc.Find("blockquote").Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())
		if text != "" {
			mu.Lock()
			markdown.WriteString("> " + text + "\n\n")
			mu.Unlock()
		}
	})

	// Process code blocks and pre elements
	doc.Find("pre, code").Each(func(i int, sel *goquery.Selection) {
		code := strings.TrimSpace(sel.Text())
		if code != "" {
			mu.Lock()
			markdown.WriteString("\n" + code + "\n\n\n")
			mu.Unlock()
		}
	})

	// Process tables
	doc.Find("table").Each(func(i int, sel *goquery.Selection) {
		mu.Lock()
		// Table header
		sel.Find("th").Each(func(j int, th *goquery.Selection) {
			markdown.WriteString("| " + th.Text() + " ")
		})
		markdown.WriteString("|\n")

		// Table separator
		sel.Find("th").Each(func(j int, th *goquery.Selection) {
			markdown.WriteString("| --- ")
		})
		markdown.WriteString("|\n")

		// Table rows
		sel.Find("tr").Each(func(j int, tr *goquery.Selection) {
			tr.Find("td").Each(func(k int, td *goquery.Selection) {
				markdown.WriteString("| " + td.Text() + " ")
			})
			markdown.WriteString("|\n")
		})
		markdown.WriteString("\n")
		mu.Unlock()
	})

	// Process images
	doc.Find("img").Each(func(i int, sel *goquery.Selection) {
		alt := sel.AttrOr("alt", "")
		src, exists := sel.Attr("src")
		if exists {
			mu.Lock()
			markdown.WriteString(fmt.Sprintf("![%s](%s)\n\n", alt, src))
			mu.Unlock()
		}
	})

	// Process definition lists
	doc.Find("dl").Each(func(i int, sel *goquery.Selection) {
		mu.Lock()
		sel.Find("dt").Each(func(j int, dt *goquery.Selection) {
			term := strings.TrimSpace(dt.Text())
			def := strings.TrimSpace(dt.Next().Text())
			markdown.WriteString(fmt.Sprintf("**%s**\n: %s\n\n", term, def))
		})
		mu.Unlock()
	})
}
func (s *Scraper) checkNoMoreURLs() {
	s.stats.Lock()
	defer s.stats.Unlock()

	// Check if we've processed all discovered links
	if len(s.urlQueue) == 0 && s.stats.processed > 0 {
		s.stats.noMoreURLs = true
		logger.Printf("No more unique URLs found. Total unique links: %d", len(s.links))
		select {
		case <-s.shutdown: // Check if shutdown is already initiated
		default:
			close(s.shutdown)
		}
	}
}

func (s *Scraper) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.shutdown:
			logger.Printf("Worker shutting down gracefully")
			return
		default:
			// Dequeue URL from Redis
			url, err := s.queue.Dequeue()
			if err != nil {
				logger.Printf("Error dequeuing URL: %v", err)
				continue
			}

			if url == "" {
				// No URLs in queue, check again after short delay
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if s.stats.processed >= s.stats.maxLinks {
				return
			}

			logger.Printf("Processing URL: %s", url)
			markdown, err := s.htmlToMarkdown(url)
			if err != nil {
				logger.Printf("Error processing %s: %v", url, err)
				s.incrementErrors()
				continue
			}
			s.writeToFile(url, markdown)
			s.incrementProcessed()
			logger.Printf("Successfully processed URL: %s", url)
		}
	}
}

func (s *Scraper) incrementProcessed() {
	s.stats.Lock()
	defer s.stats.Unlock()

	s.stats.processed++
	if s.stats.processed >= s.stats.maxLinks {
		logger.Printf("Reached maximum link count of %d, initiating shutdown...", s.stats.maxLinks)
		select {
		case <-s.shutdown: // Check if shutdown is already initiated
		default:
			close(s.shutdown)
		}
	}
}
func (s *Scraper) incrementErrors() {
	s.stats.Lock()
	s.stats.errors++
	s.stats.Unlock()
}


func (s *Scraper) writeToFile(urlStr, markdown string) {
	filename := fmt.Sprintf("output/%s.md", url.QueryEscape(urlStr))
	os.MkdirAll("output", 0755)

	file, err := os.Create(filename)
	if err != nil {
		logger.Printf("Error creating file for %s: %v", urlStr, err)
		return
	}
	defer file.Close()

	if _, err := file.WriteString(markdown); err != nil {
		logger.Printf("Error writing to file for %s: %v", urlStr, err)
	}

}

func (s *Scraper) printStats() {
	duration := time.Since(s.startTime)
	logger.Printf("\nScraping completed:")
	logger.Printf("Total URLs processed: %d", s.stats.processed)
	logger.Printf("Total errors encountered: %d", s.stats.errors)
	logger.Printf("Total unique links found: %d", len(s.links))
	logger.Printf("Total execution time: %v", duration)
	logger.Printf("Average time per URL: %v", duration/time.Duration(s.stats.processed))
}

// Add this function to implement exponential backoff
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

			logger.Printf("Rate limited (attempt %d/%d). Waiting %v before retry...",
				attempt+1, maxRetries, delay)

			res.Body.Close()
			time.Sleep(delay)
			continue
		}

		return res, nil
	}

	return nil, fmt.Errorf("max retries exceeded while handling rate limits")
}

func handleSingleURLScrape(c *gin.Context) {
	urlString := c.Query("url")
	if urlString == "" {
		c.JSON(400, gin.H{"error": "url parameter is required"})
		return
	}

	scraper := NewScraper(1, urlString, 1)
	markdown, err := scraper.htmlToMarkdown(urlString)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("output/%s.md", url.QueryEscape(urlString))
	scraper.writeToFile(urlString, markdown)

	c.JSON(200, gin.H{
		"status":      "success",
		"url":         urlString,
		"output_file": filename,
	})
}

func handleFullScrape(c *gin.Context) {
	rootURL := c.Query("rootUrl")
	maxURLs := c.DefaultQuery("maxUrls", "100")
	concurrency := c.DefaultQuery("concurrency", "10")

	maxURLsInt, _ := strconv.Atoi(maxURLs)
	concurrencyInt, _ := strconv.Atoi(concurrency)

	if rootURL == "" {
		c.JSON(400, gin.H{"error": "rootUrl parameter is required"})
		return
	}

	scraper := NewScraper(concurrencyInt, rootURL, maxURLsInt)

	// Run scraper in goroutine to not block the response
	go func() {
		scraper.Start(rootURL)
	}()

	c.JSON(200, gin.H{
		"status":      "started",
		"root_url":    rootURL,
		"max_urls":    maxURLsInt,
		"concurrency": concurrencyInt,
	})
}

func main() {
	startURL := "https://www.wpelemento.com/"

	logger.Printf("Initializing scraper...")
	scraper := NewScraper(200, startURL, 50)

	scraper.Start(startURL)
}
