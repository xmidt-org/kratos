package kratos

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
)

// outboundSender provides a way to send wrps.
type outboundSender interface {
	Send([]byte)
	Close()
}

// senderQueue implements the outboundSender, allowing for asynchronous sending
// through a websocket connection.
type senderQueue struct {
	incoming   chan []byte
	connection websocketConnection
	workers    *semaphore.Weighted
	wg         sync.WaitGroup
	logger     *zap.Logger
	once       sync.Once
	closed     atomic.Value
}

// NewSender creates a new senderQueue with the given websocketConnection and
// other configuration.
func NewSender(connection websocketConnection, maxWorkers int, queueSize int, logger *zap.Logger) *senderQueue {
	size := queueSize
	if size < minQueueSize {
		size = minQueueSize
	}
	numWorkers := maxWorkers
	if numWorkers < minWorkers {
		numWorkers = minWorkers
	}
	s := senderQueue{
		incoming:   make(chan []byte, size),
		connection: connection,
		workers:    semaphore.NewWeighted(int64(numWorkers)),
		logger:     logger,
	}
	s.wg.Add(1)
	go s.startSending()
	return &s
}

// Send adds the message given to the queue of messages to be sent.
func (s *senderQueue) Send(msg []byte) {
	switch s.closed.Load() {
	case true:
		s.logger.Error("Failed to queue message. SenderWorker is no longer accepting messages.")
	default:
		s.incoming <- msg
	}
}

// Close provides a way to gracefully stop the senderQueue.  It stops receiving
// any new messages to send and then waits until all messages have been sent.
func (s *senderQueue) Close() {
	s.once.Do(func() {
		s.closed.Store(true)
		close(s.incoming)
		s.wg.Wait()
	})
}

// startSending is called when the senderQueue is created, allowing the queue
// to read the incoming messages and send them.
func (s *senderQueue) startSending() {
	ctx := context.Background()
	defer s.wg.Done()
	for i := range s.incoming {
		s.workers.Acquire(ctx, 1)
		s.wg.Add(1)
		go s.send(i)
	}
}

// send takes the incoming message and actually sends it.
func (s *senderQueue) send(incoming []byte) {
	defer s.wg.Done()
	defer s.workers.Release(1)

	s.logger.Debug("Sending message...")

	err := s.connection.WriteMessage(websocket.BinaryMessage, incoming)
	if err != nil {
		s.logger.Error("Failed to send message",
			zap.Error(err),
			zap.String("msg", string(incoming)))
		return
	}

	s.logger.Debug("Message Sent")
}
