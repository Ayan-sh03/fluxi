package scrapper

import (
	"fmt"
	"net/url"
	"os"
	"scrapper/pkg/logger"
	"strings"
)

func (s *Scraper) isValidURL(urlString string) bool {

	if urlString == "" {
		logger.Logger.Printf("Rejected: Empty URL")
		return false
	}

	if strings.HasPrefix(urlString, "#") {
		logger.Logger.Printf("Rejected: URL starts with #")
		return false
	}

	u, err := url.Parse(urlString)
	if err != nil {
		logger.Logger.Printf("Rejected: URL parsing failed - %v", err)
		return false
	}

	// logger.Logger.Printf("URL Analysis - Root Domain: %s, Current Host: %s, Root Path: %s, Current Path: %s",
	// s.rootDomain, u.Host, s.rootPath, u.Path)

	if u.Host != s.rootDomain && u.Host != "" {
		// logger.Logger.Printf("Rejected: Domain mismatch - Expected: %s, Got: %s", s.rootDomain, u.Host)
		return false
	}

	if s.rootPath != "" && s.rootPath != "/" {
		hasPrefix := strings.HasPrefix(u.Path, s.rootPath)
		// logger.Logger.Printf("Path check - Required Prefix: %s, Has Prefix: %v", s.rootPath, hasPrefix)
		return hasPrefix
	}

	return u.Host == s.rootDomain || u.Host == ""
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

func (s *Scraper) incrementErrors() {
	s.stats.errors.Add(1)
}
func (s *Scraper) WriteToFile(urlStr, markdown string) {
	filename := fmt.Sprintf("output/%s.md", url.QueryEscape(urlStr))
	os.MkdirAll("output", 0755)

	file, err := os.Create(filename)
	if err != nil {
		logger.Logger.Printf("Error creating file for %s: %v", urlStr, err)
		return
	}
	defer file.Close()

	if _, err := file.WriteString(markdown); err != nil {
		logger.Logger.Printf("Error writing to file for %s: %v", urlStr, err)
	}

}
