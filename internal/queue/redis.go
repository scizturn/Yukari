package queue

import (
	"context"

	"github.com/kyou-id/yukari/internal/domain"
	"github.com/redis/go-redis/v9"
)

type RedisQueue struct {
	client *redis.Client
	name   string
}

func NewRedisQueue(addr, password string, db int, name string) *RedisQueue {
	if name == "" {
		name = "birthday_email_jobs"
	}
	return &RedisQueue{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		}),
		name: name,
	}
}

func (q *RedisQueue) Close() error {
	return q.client.Close()
}

func (q *RedisQueue) Enqueue(ctx context.Context, job domain.BirthdayJob) error {
	payload, err := EncodeBirthdayJob(job)
	if err != nil {
		return err
	}
	return q.client.RPush(ctx, q.name, payload).Err()
}

func (q *RedisQueue) EnqueueAnniversaryTo(ctx context.Context, queueName string, job domain.AnniversaryJob) error {
	payload, err := EncodeAnniversaryJob(job)
	if err != nil {
		return err
	}
	return q.client.RPush(ctx, queueName, payload).Err()
}
