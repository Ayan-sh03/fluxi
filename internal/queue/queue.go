package queue

import "time"

type QueueManager interface {
	Enqueue(url string) error
	Dequeue() (string, error)
}

type QueueItem struct {
	URL       string    `json:"url"`
	Timestamp time.Time `json:"timestamp"`
	Retries   int       `json:"retries"`
	Priority  int       `json:"priority"`
}
