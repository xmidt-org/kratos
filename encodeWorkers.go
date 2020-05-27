package kratos

import (
	"bytes"
	"sync"
	"sync/atomic"

	"github.com/go-kit/kit/log"
	"github.com/goph/emperror"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/wrp"
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
	workers  semaphore.Interface
	wg       sync.WaitGroup
	logger   log.Logger
	once     sync.Once
	closed   atomic.Value
}

// NewEncoderSender creates a new encoderQueue, that allows for asynchronous
// sending outbound.
func NewEncoderSender(sender outboundSender, maxWorkers int, queueSize int, logger log.Logger) *encoderQueue {
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
		workers:  semaphore.New(numWorkers),
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
		logging.Error(e.logger).Log(logging.MessageKey(),
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
	for i := range e.incoming {
		e.workers.Acquire()
		e.wg.Add(1)
		go e.parse(i)
	}
}

// parse encodes the wrp message and then uses the outboundSender to send it.
func (e *encoderQueue) parse(incoming *wrp.Message) {
	defer e.wg.Done()
	defer e.workers.Release()
	var buffer bytes.Buffer

	// encoding
	logging.Debug(e.logger).Log(logging.MessageKey(), "Encoding message...")
	err := wrp.NewEncoder(&buffer, wrp.Msgpack).Encode(incoming)
	if err != nil {
		logging.Error(e.logger, emperror.Context(err)...).
			Log(logging.MessageKey(), "Failed to encode message",
				logging.ErrorKey(), err.Error(),
				"message", incoming)
		return
	}
	logging.Debug(e.logger).Log(logging.MessageKey(), "Message Encoded")

	// sending
	e.sender.Send(buffer.Bytes())
	logging.Debug(e.logger).Log(logging.MessageKey(), "Message Sent")
}
