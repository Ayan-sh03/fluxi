package scrapper

import (
	"fmt"
	"net/http"
	"scrapper/pkg/logger"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

func (s *Scraper) HtmlToText(url string) (string, error) {

	//check if maxLinks reached
	if s.stats.processed.Load() >= int64(s.stats.maxLinks) {
		s.cancel()
		return "", fmt.Errorf("max links reached")
	}

	req, err := http.NewRequest("GET", url, nil)

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

	var text strings.Builder
	var wg sync.WaitGroup

	// Process different HTML elements concurrently
	wg.Add(4)

	// Process headings
	go func() {
		defer wg.Done()
		s.processHeadingsText(doc, &text)
	}()

	// Process paragraphs and text
	go func() {
		defer wg.Done()
		s.processTextText(doc, &text)
	}()

	// Process links
	go func() {
		defer wg.Done()
		s.processLinksText(doc, url)
	}()

	// Process other elements
	go func() {
		defer wg.Done()
		s.processOtherElementsText(doc, &text)
	}()

	wg.Wait()

	return strings.TrimSpace(text.String()), nil
}

func (s *Scraper) processHeadingsText(doc *goquery.Document, text *strings.Builder) {
	var mu sync.Mutex
	doc.Find("h1, h2, h3, h4, h5, h6").Each(func(i int, sel *goquery.Selection) {
		headingText := strings.TrimSpace(sel.Text())
		if headingText != "" {
			mu.Lock()
			text.WriteString(headingText + "\n\n")
			mu.Unlock()
		}
	})
}

func (s *Scraper) processTextText(doc *goquery.Document, text *strings.Builder) {
	var mu sync.Mutex
	doc.Find("p").Each(func(i int, sel *goquery.Selection) {
		t := strings.TrimSpace(sel.Text())
		if t != "" {
			mu.Lock()
			text.WriteString(t + "\n\n")
			mu.Unlock()
		}
	})
}

func (s *Scraper) processLinksText(doc *goquery.Document, baseURL string) {
	doc.Find("a").Each(func(i int, sel *goquery.Selection) {
		href, exists := sel.Attr("href")
		if !exists {
			return
		}

		absoluteURL := s.makeAbsoluteURL(href, baseURL)
		if s.isValidURL(absoluteURL) {
			if _, loaded := s.visited.LoadOrStore(absoluteURL, true); !loaded {
				if s.stats.processed.Load() >= int64(s.stats.maxLinks) {
					// Do not enqueue new URLs if maxLinks is reached
					return
				}
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

func (s *Scraper) processOtherElementsText(doc *goquery.Document, text *strings.Builder) {
	var mu sync.Mutex

	// Process blockquotes
	doc.Find("blockquote").Each(func(i int, sel *goquery.Selection) {
		t := strings.TrimSpace(sel.Text())
		if t != "" {
			mu.Lock()
			text.WriteString(t + "\n\n")
			mu.Unlock()
		}
	})

	// Process code blocks and pre elements
	doc.Find("pre, code").Each(func(i int, sel *goquery.Selection) {
		code := strings.TrimSpace(sel.Text())
		if code != "" {
			mu.Lock()
			text.WriteString(code + "\n\n")
			mu.Unlock()
		}
	})

	// Process tables
	doc.Find("table").Each(func(i int, sel *goquery.Selection) {
		mu.Lock()
		// Table rows
		sel.Find("tr").Each(func(j int, tr *goquery.Selection) {
			tr.Find("th, td").Each(func(k int, td *goquery.Selection) {
				text.WriteString(td.Text() + "\t")
			})
			text.WriteString("\n")
		})
		text.WriteString("\n")
		mu.Unlock()
	})

	// Process images
	doc.Find("img").Each(func(i int, sel *goquery.Selection) {
		alt := sel.AttrOr("alt", "")
		src, exists := sel.Attr("src")
		if exists {
			mu.Lock()
			text.WriteString(fmt.Sprintf("Image: %s (Source: %s)\n\n", alt, src))
			mu.Unlock()
		}
	})

	// Process definition lists
	doc.Find("dl").Each(func(i int, sel *goquery.Selection) {
		mu.Lock()
		sel.Find("dt").Each(func(j int, dt *goquery.Selection) {
			term := strings.TrimSpace(dt.Text())
			def := strings.TrimSpace(dt.Next().Text())
			text.WriteString(fmt.Sprintf("%s: %s\n\n", term, def))
		})
		mu.Unlock()
	})
}
