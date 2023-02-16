package kratos

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"

	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
)

// encoderSender is anything that can encode and send a message.
type encoderSender interface {
	EncodeAndSend(*wrp.Message)
	Close()
}

// encoderQueue implements an asynchronous encoderSender.
type encoderQueue struct {
	incoming chan *wrp.Message
	sender   outboundSender
	workers  *semaphore.Weighted
	wg       sync.WaitGroup
	logger   *zap.Logger
	once     sync.Once
	closed   atomic.Value
}

// NewEncoderSender creates a new encoderQueue, that allows for asynchronous
// sending outbound.
func NewEncoderSender(sender outboundSender, maxWorkers int, queueSize int, logger *zap.Logger) *encoderQueue {
	size := queueSize
	if size < minQueueSize {
		size = minQueueSize
	}
	numWorkers := maxWorkers
	if numWorkers < minWorkers {
		numWorkers = minWorkers
	}
	e := encoderQueue{
		incoming: make(chan *wrp.Message, size),
		sender:   sender,
		workers:  semaphore.NewWeighted(int64(numWorkers)),
		logger:   logger,
	}
	e.wg.Add(1)
	go e.startParsing()
	return &e
}

// EncodeAndSend adds the message to the queue to be sent.  It will block if
// the queue is full.  This should not be called after Close().
//TODO: we should consider returning an error in the case in which we can no longer encode
func (e *encoderQueue) EncodeAndSend(msg *wrp.Message) {
	switch e.closed.Load() {
	case true:
		e.logger.Error(
			"Failed to queue message. EncoderQueue is no longer accepting messages.")
	default:
		e.incoming <- msg
	}
}

// Close closes the queue, not allowing any more messages to be sent.  Then
// it will block until all the messages in the queue have been sent.
func (e *encoderQueue) Close() {
	e.once.Do(func() {
		e.closed.Store(true)
		close(e.incoming)
		e.wg.Wait()
		e.sender.Close()
	})
}

// startParsing is called when the encoderQueue is created.  It is a
// long-running go routine that starts workers to parse and send messages as
// they arrive in the queue.
func (e *encoderQueue) startParsing() {
	defer e.wg.Done()
	ctx := context.Background()  // TODO - does this need to be withCancel?
	
	for i := range e.incoming {
		e.workers.Acquire(ctx, 1)
		e.wg.Add(1)
		go e.parse(i)
	}
}

// parse encodes the wrp message and then uses the outboundSender to send it.
func (e *encoderQueue) parse(incoming *wrp.Message) {
	defer e.wg.Done()
	defer e.workers.Release(1)
	var buffer bytes.Buffer

	// encoding
	e.logger.Error("Encoding message...")
	err := wrp.NewEncoder(&buffer, wrp.Msgpack).Encode(incoming)
	if err != nil {
		e.logger.Error( "Failed to encode message", zap.Error(err),
				zap.Any("message", incoming))
		return
	}
	e.logger.Debug("Message Encoded")

	// sending
	e.sender.Send(buffer.Bytes())
	e.logger.Debug("Message Sent")
}
