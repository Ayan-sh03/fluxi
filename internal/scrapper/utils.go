package scrapper

import (
	"net/url"
	"strings"
)

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
