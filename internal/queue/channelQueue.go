package queue

type ChannelQueue struct {
	queue chan string
}

func (q *ChannelQueue) Enqueue(url string) error {
	q.queue <- url
	return nil
}

func (q *ChannelQueue) Clear() error {
	for len(q.queue) > 0 {
		<-q.queue
	}
	return nil
}

func (q *ChannelQueue) Dequeue() (string, error) {
	select {
	case url := <-q.queue:
		return url, nil
	default:
		return "", nil
	}
}

func (q *ChannelQueue) Len() (int, error) {
	return len(q.queue), nil
}

func NewChannelQueue(capacity int) *ChannelQueue {
	return &ChannelQueue{
		queue: make(chan string, capacity),
	}
}
