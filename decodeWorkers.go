// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package kratos

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
)

const (
	minWorkers   = 1
	minQueueSize = 1
)

// decoderSender is anything that decodes a message from bytes and then sends
// it downstream.
type decoderSender interface {
	DecodeAndSend([]byte)
	Close()
}

// decoderQueue implements an asynchronous decoderSender.
type decoderQueue struct {
	incoming chan []byte
	sender   registryHandler
	workers  *semaphore.Weighted
	wg       sync.WaitGroup
	logger   *zap.Logger
	once     sync.Once
	closed   atomic.Value
}

// NewDecoderSender creates a new decoderQueue for decoding and sending
// messages.
func NewDecoderSender(sender registryHandler, maxWorkers int, queueSize int, logger *zap.Logger) *decoderQueue {
	size := queueSize
	if size < minQueueSize {
		size = minQueueSize
	}
	numWorkers := maxWorkers
	if numWorkers < minWorkers {
		numWorkers = minWorkers
	}
	d := decoderQueue{
		incoming: make(chan []byte, size),
		sender:   sender,
		workers:  semaphore.NewWeighted(int64(numWorkers)),
		logger:   logger,
	}
	d.wg.Add(1)
	go d.startParsing()
	return &d
}

// DecodeAndSend places the message on the queue.  This will block when the
// queue is full.  This should not be called after Close().
func (d *decoderQueue) DecodeAndSend(msg []byte) {
	switch d.closed.Load() {
	case true:
		d.logger.Error(
			"Failed to queue message. DecoderQueue is no longer accepting messages.")
	default:
		d.incoming <- msg
	}
}

// Close stops consumers from being able to add new messages to be decoded.
// Then it blocks until all messages have been decoded and sent.
func (d *decoderQueue) Close() {
	d.once.Do(func() {
		d.closed.Store(true)
		close(d.incoming)
		d.wg.Wait()
		d.sender.Close()
	})
}

// startParsing is called when the decoderQueue is created.  It is a
// long-running go routine that watches the queue and starts other go routines
// to decode and send the messages.
func (d *decoderQueue) startParsing() {
	ctx := context.Background()
	defer d.wg.Done()
	for i := range d.incoming {
		d.workers.Acquire(ctx, 1)
		d.wg.Add(1)
		go d.parse(i)
	}
}

// parse is called to decode and then send an incoming message, using the
// registryHandler.
func (d *decoderQueue) parse(incoming []byte) {
	defer d.wg.Done()
	defer d.workers.Release(1)
	msg := wrp.Message{}

	// decoding
	d.logger.Debug("Decoding message...")
	err := wrp.NewDecoderBytes(incoming, wrp.Msgpack).Decode(&msg)
	if err != nil {
		d.logger.Error("Failed to decode message into wrp", zap.Error(err))
		return
	}
	d.logger.Debug("Message Decoded")

	// sending
	d.sender.GetHandlerThenSend(&msg)
	d.logger.Debug("Message Sent")
}
