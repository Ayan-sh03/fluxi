package main

import "scrapper/pkg/logger"

func main() {
	logger.Init()

	startUrl := "https://www.wpelemento.com/"

	scraper := NewScraper(200, startUtl, 50)

}