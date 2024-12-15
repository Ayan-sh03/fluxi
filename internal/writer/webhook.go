package writer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"math/rand"
)

type WebhookWriter struct {
	WebhookURL string
	client     *http.Client
	maxRetries int
}

type Body struct {
	JobId   string `json:"job_id"`
	RootUrl string `json:"root_url"`
	Url     string `json:"url"`
	Data    string `json:"data"`
}

func NewWebhookWriter(webhookURL string) *WebhookWriter {
	return &WebhookWriter{
		WebhookURL: webhookURL,
		maxRetries: 3,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:      10,
				IdleConnTimeout:   30 * time.Second,
				DisableKeepAlives: false,
			},
		},
	}
}

func (w *WebhookWriter) Write(jobId, rootUrl, url, data string) error {
	body := Body{
		JobId:   jobId,
		RootUrl: rootUrl,
		Url:     url,
		Data:    data,
	}
	log.Printf("Starting webhook write for job %s to %s", jobId, url)
	defer log.Printf("Completed webhook write for job %s", jobId)

	jsonData, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal error: %v", err)
	}

	var lastErr error
	for attempt := 0; attempt <= w.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter to prevent thundering herd
			backoff := time.Duration(attempt) * time.Second
			jitter := time.Duration(rand.Int63n(1000)) * time.Millisecond
			time.Sleep(backoff + jitter)
		}
		err = w.doRequest(jsonData)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("failed after %d attempts, last error: %v", w.maxRetries, lastErr)
}

func (w *WebhookWriter) doRequest(jsonData []byte) error {
	req, err := http.NewRequest(http.MethodPost, w.WebhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("request creation error: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Scraper-Webhook-Client/1.0")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Read and discard response body to reuse connection
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
