package queue

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis"
)

type RedisQueue struct {
	client   *redis.Client
	key      string
	maxLen   int
	ctx      context.Context
	isClosed atomic.Bool
}

func NewRedisQueue(addr string, urlString string, maxLen int) (*RedisQueue, error) {
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
		maxLen: maxLen,
		ctx:    ctx,
	}, nil
}

func (q *RedisQueue) Clear() error {
	// fmt.Println("Clearng Redis queue :", q.key)
	return q.client.Del(q.key).Err()
}
func (q *RedisQueue) Enqueue(urlDataJSON string) error {
	if q.isClosed.Load() {
		return fmt.Errorf("queue is closed: maximum limit reached")
	}

	currentLen, err := q.Len()
	if err != nil {
		return err
	}

	if currentLen >= q.maxLen {
		q.isClosed.Store(true)
		return fmt.Errorf("queue at capacity: %d urls", q.maxLen)
	}

	// Store the JSON string directly
	err = q.client.LPush(q.key, urlDataJSON).Err()
	if err != nil {
		return err
	}

	return nil
}

func (q *RedisQueue) Dequeue() (string, error) {
	result, err := q.client.RPop(q.key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	// Return the JSON string directly
	return result, nil
}

func (q *RedisQueue) Len() (int, error) {
	count, err := q.client.LLen(q.key).Result()
	if err != nil {
		return 0, err
	}
	return int(count), nil
}
