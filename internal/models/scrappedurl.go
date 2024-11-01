package models

type ScrappedUrl struct {
	JobId        string   `json:"job_id"`
	ScrappedUrls []string `json:"scrapped_urls"`
	Status       string   `json:"status"`
}
