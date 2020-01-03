package kratos

import (
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/goph/emperror"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/wrp"
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
	workers  semaphore.Interface
	wg       sync.WaitGroup
	logger   log.Logger
}

// NewDecoderSender creates a new decoderQueue for decoding and sending
// messages.
func NewDecoderSender(sender registryHandler, maxWorkers int, queueSize int, logger log.Logger) *decoderQueue {
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
		workers:  semaphore.New(numWorkers),
		logger:   logger,
	}
	d.wg.Add(1)
	go d.startParsing()
	return &d
}

// DecodeAndSend places the message on the queue.  This will block when the
// queue is full.  This should not be called after Close().
func (d *decoderQueue) DecodeAndSend(msg []byte) {
	d.incoming <- msg
}

// Close stops consumers from being able to add new messages to be decoded.
// Then it blocks until all messages have been decoded and sent.
func (d *decoderQueue) Close() {
	close(d.incoming)
	d.wg.Wait()
	d.sender.Close()
}

// startParsing is called when the decoderQueue is created.  It is a
// long-running go routine that watches the queue and starts other go routines
// to decode and send the messages.
func (d *decoderQueue) startParsing() {
	defer d.wg.Done()
	for i := range d.incoming {
		d.workers.Acquire()
		d.wg.Add(1)
		go d.parse(i)
	}
}

// parse is called to decode and then send an incoming message, using the
// registryHandler.
func (d *decoderQueue) parse(incoming []byte) {
	defer d.wg.Done()
	defer d.workers.Release()
	msg := wrp.Message{}

	// decoding
	logging.Debug(d.logger).Log(logging.MessageKey(), "Decoding message...")
	err := wrp.NewDecoderBytes(incoming, wrp.Msgpack).Decode(&msg)
	if err != nil {
		logging.Error(d.logger, emperror.Context(err)...).
			Log(logging.MessageKey(), "Failed to decode message into wrp", logging.ErrorKey(), err.Error())
		return
	}
	logging.Debug(d.logger).Log(logging.MessageKey(), "Message Decoded")

	// sending
	d.sender.GetHandlerThenSend(&msg)
	logging.Debug(d.logger).Log(logging.MessageKey(), "Message Sent")

}
