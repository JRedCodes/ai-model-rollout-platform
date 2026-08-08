package ingestion

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/JRedCodes/rollout-controller/internal/metrics"
)

type Consumer struct {
	rdb           *redis.Client
	streamKey     string
	consumerGroup string
	consumerName  string
	store         *metrics.Store
}

func New(rdb *redis.Client, streamKey, consumerGroup, consumerName string, store *metrics.Store) *Consumer {
	return &Consumer{
		rdb:           rdb,
		streamKey:     streamKey,
		consumerGroup: consumerGroup,
		consumerName:  consumerName,
		store:         store,
	}
}

type streamEvent struct {
	Success   bool `json:"success"`
	LatencyMs int  `json:"latencyMs"`
}

func (c *Consumer) Run(ctx context.Context) {
	if err := c.ensureConsumerGroup(ctx); err != nil {
		log.Printf("ingestion: failed to create consumer group: %v", err)
		return
	}

	log.Printf("ingestion: listening on stream %s (group: %s)", c.streamKey, c.consumerGroup)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.consumerGroup,
			Consumer: c.consumerName,
			Streams:  []string{c.streamKey, ">"},
			Count:    100,
			Block:    2 * time.Second,
		}).Result()

		if err != nil {
			if err == context.Canceled || err == redis.Nil {
				continue
			}
			log.Printf("ingestion: xreadgroup error: %v", err)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				c.process(ctx, msg)
			}
		}
	}
}

func (c *Consumer) process(ctx context.Context, msg redis.XMessage) {
	raw, ok := msg.Values["event"].(string)
	if !ok {
		c.ack(ctx, msg.ID)
		return
	}

	var event streamEvent
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		log.Printf("ingestion: failed to parse event %s: %v", msg.ID, err)
		c.ack(ctx, msg.ID)
		return
	}

	c.store.Record(metrics.EventRecord{
		Timestamp: time.Now(),
		Success:   event.Success,
		LatencyMs: event.LatencyMs,
	})

	c.ack(ctx, msg.ID)
}

func (c *Consumer) ack(ctx context.Context, id string) {
	if err := c.rdb.XAck(ctx, c.streamKey, c.consumerGroup, id).Err(); err != nil {
		log.Printf("ingestion: failed to ack message %s: %v", id, err)
	}
}

func (c *Consumer) ensureConsumerGroup(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, c.streamKey, c.consumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}
