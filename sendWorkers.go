package kratos

import (
	"sync"
	"sync/atomic"

	"github.com/go-kit/kit/log"
	"github.com/goph/emperror"
	"github.com/gorilla/websocket"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/semaphore"
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
	workers    semaphore.Interface
	wg         sync.WaitGroup
	logger     log.Logger
	once       sync.Once
	closed     atomic.Value
}

// NewSender creates a new senderQueue with the given websocketConnection and
// other configuration.
func NewSender(connection websocketConnection, maxWorkers int, queueSize int, logger log.Logger) *senderQueue {
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
		workers:    semaphore.New(numWorkers),
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
		logging.Error(s.logger).Log(logging.MessageKey(),
			"Failed to queue message. SenderWorker is no longer accepting messages.")
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
	defer s.wg.Done()
	for i := range s.incoming {
		s.workers.Acquire()
		s.wg.Add(1)
		go s.send(i)
	}
}

// send takes the incoming message and actually sends it.
func (s *senderQueue) send(incoming []byte) {
	defer s.wg.Done()
	defer s.workers.Release()

	logging.Debug(s.logger).Log(logging.MessageKey(), "Sending message...")

	err := s.connection.WriteMessage(websocket.BinaryMessage, incoming)
	if err != nil {
		logging.Error(s.logger, emperror.Context(err)...).
			Log(logging.MessageKey(), "Failed to send message",
				logging.ErrorKey(), err.Error(),
				"msg", string(incoming))
		return
	}

	logging.Debug(s.logger).Log(logging.MessageKey(), "Message Sent")
}
