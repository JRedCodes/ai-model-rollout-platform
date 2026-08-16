package ingestion

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/JRedCodes/rollout-controller/internal/batchlogger"
	"github.com/JRedCodes/rollout-controller/internal/metrics"
)

type Consumer struct {
	rdb           *redis.Client
	streamKey     string
	consumerGroup string
	consumerName  string
	rolloutID     string
	store         *metrics.Store
	logger        *batchlogger.BatchLogger
}

// New builds a consumer scoped to one tenant's rollout. Even though each
// tenant gets its own consumer group (so tenants' consumers don't compete
// over message distribution), every group still independently sees the
// *entire* shared stream -- Redis Streams consumer groups have no
// server-side content filtering. rolloutID is what process() uses to
// discard every other tenant's events client-side.
func New(
	rdb *redis.Client,
	streamKey, consumerGroup, consumerName, rolloutID string,
	store *metrics.Store,
	logger *batchlogger.BatchLogger,
) *Consumer {
	return &Consumer{
		rdb:           rdb,
		streamKey:     streamKey,
		consumerGroup: consumerGroup,
		consumerName:  consumerName,
		rolloutID:     rolloutID,
		store:         store,
		logger:        logger,
	}
}

type streamEvent struct {
	EventID        string  `json:"eventId"`
	RequestID      string  `json:"requestId"`
	UserID         string  `json:"userId"`
	RolloutID      *string `json:"rolloutId"`
	RolloutPhaseID *string `json:"rolloutPhaseId"`
	ModelVersionID string  `json:"modelVersionId"`
	Assignment     string  `json:"assignment"`
	Success        bool    `json:"success"`
	ErrorType      *string `json:"errorType"`
	LatencyMs      int     `json:"latencyMs"`
	OccurredAt     string  `json:"occurredAt"`
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

	// Not this pipeline's rollout -- another tenant's event, seen only
	// because consumer groups can't filter server-side. Ack it so this
	// group's cursor advances (some other tenant's own consumer group is
	// independently seeing and recording the same message), but don't
	// record it into this store or enqueue it -- the batch logger is a
	// single shared instance, so enqueuing here too would double-insert.
	if event.RolloutID == nil || *event.RolloutID != c.rolloutID {
		c.ack(ctx, msg.ID)
		return
	}

	c.store.Record(metrics.EventRecord{
		Timestamp: time.Now(),
		Success:   event.Success,
		LatencyMs: event.LatencyMs,
	})

	occurredAt, err := time.Parse(time.RFC3339, event.OccurredAt)
	if err != nil {
		occurredAt = time.Now()
	}

	c.logger.Enqueue(batchlogger.EventRow{
		ID:             event.EventID,
		RequestID:      event.RequestID,
		UserID:         event.UserID,
		RolloutID:      event.RolloutID,
		RolloutPhaseID: event.RolloutPhaseID,
		ModelVersionID: event.ModelVersionID,
		Assignment:     event.Assignment,
		Success:        event.Success,
		ErrorType:      event.ErrorType,
		LatencyMs:      event.LatencyMs,
		OccurredAt:     occurredAt,
	})

	c.ack(ctx, msg.ID)
}

func (c *Consumer) ack(ctx context.Context, id string) {
	if err := c.rdb.XAck(ctx, c.streamKey, c.consumerGroup, id).Err(); err != nil {
		log.Printf("ingestion: failed to ack message %s: %v", id, err)
	}
}

func (c *Consumer) ensureConsumerGroup(ctx context.Context) error {
	// "$" -- only messages published after this group is created. Each
	// tenant's consumer group is created the first time its rollout
	// activates, which can be long after the shared stream itself started
	// accumulating other tenants' history; starting from "0" would replay
	// all of that pre-existing history into a tenant that never asked for it.
	err := c.rdb.XGroupCreateMkStream(ctx, c.streamKey, c.consumerGroup, "$").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}
