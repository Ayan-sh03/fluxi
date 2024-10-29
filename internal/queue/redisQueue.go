package queue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis"
)

type RedisQueue struct {
	client *redis.Client
	key    string
	ctx    context.Context
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
