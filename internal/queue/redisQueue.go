package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis"
)

type RedisQueue struct {
	client *redis.Client
	key    string
	ctx    context.Context
}

func NewRedisQueue(addr string, urlString string) (*RedisQueue, error) {
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
	fmt.Println("Initialised Redis Queue for key ", "scraper:urls:"+urlString)
	return &RedisQueue{
		client: client,
		key:    "scraper:urls:" + urlString,
		ctx:    ctx,
	}, nil
}

func (q *RedisQueue) Clear() error {
	// fmt.Println("Clearng Redis queue :", q.key)
	return q.client.Del(q.key).Err()
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
	err = q.client.LPush(q.key, data).Err()
	if err != nil {
		return err
	}
	// fmt.Println("Enqueued URL:", url)
	// _, err := q.client.LRange(q.key, 0, -1).Result()
	// fmt.Println("All URLs in queue:", q.key)

	return err
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

func (q *RedisQueue) Len() (int, error) {
	count, err := q.client.LLen(q.key).Result()
	if err != nil {
		return 0, err
	}
	return int(count), nil
}
