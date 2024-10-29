package scrapper

import (
	"fmt"
	"net/http"
	"scrapper/pkg/logger"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func (s *Scraper) HtmlToMarkdown(url string) (string, error) {
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
					logger.Logger.Printf("Error enqueueing URL: %v", err)
				}
			}
		}
	})
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
