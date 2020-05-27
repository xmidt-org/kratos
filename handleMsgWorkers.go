package kratos

import (
	"sync"
	"sync/atomic"

	"github.com/go-kit/kit/log"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/wrp"
)

// downstreamSender sends wrp messages to components downstream.
type downstreamSender interface {
	Send(DownstreamHandler, *wrp.Message)
	Close()
}

// sendInfo dictates the handler and the message it should receive.
type sendInfo struct {
	handler DownstreamHandler
	msg     *wrp.Message
}

// downstreamSenderQueue implements an ascynhronous downstreamSender.  Messages
// to be sent are placed on a queue and then sent when the resources are
// available.
type downstreamSenderQueue struct {
	incoming chan sendInfo
	sendFunc sendWRPFunc
	workers  semaphore.Interface
	wg       sync.WaitGroup
	logger   log.Logger
	once     sync.Once
	closed   atomic.Value
}

// NewDownstreamSender creates a new downstreamSenderQueue for asynchronously
// sending wrp messages downstream.
func NewDownstreamSender(senderFunc sendWRPFunc, maxWorkers int, queueSize int, logger log.Logger) *downstreamSenderQueue {
	size := queueSize
	if size < minQueueSize {
		size = minQueueSize
	}
	numWorkers := maxWorkers
	if numWorkers < minWorkers {
		numWorkers = minWorkers
	}
	d := downstreamSenderQueue{
		incoming: make(chan sendInfo, size),
		sendFunc: senderFunc,
		workers:  semaphore.New(numWorkers),
		logger:   logger,
	}
	d.wg.Add(1)
	go d.startSending()
	return &d
}

// Send adds the wrp message and the handler to use for it to the queue of
// messages to be sent.  It will block if the queue is full.  This should not
// be called after Close().
func (d *downstreamSenderQueue) Send(handler DownstreamHandler, msg *wrp.Message) {
	switch d.closed.Load() {
	case true:
		logging.Error(d.logger).Log(logging.MessageKey(),
			"Failed to queue message. DownstreamSenderQueue is no longer accepting messages.")
	default:
		d.incoming <- sendInfo{handler: handler, msg: msg}
	}
}

// Close closes the queue channel and then blocks until all remaining messages
// have been sent.
func (d *downstreamSenderQueue) Close() {
	d.once.Do(func() {
		d.closed.Store(true)
		close(d.incoming)
		d.wg.Wait()
	})
}

// startSending is called when the downstreamSenderQueue is created.  It is a
// long-running goroutine that watches the incoming messages queue and spawns
// workers to send them.
func (d *downstreamSenderQueue) startSending() {
	defer d.wg.Done()
	for i := range d.incoming {
		d.workers.Acquire()
		d.wg.Add(1)
		go d.send(i)
	}
}

// send calls HandleMessage() on the handler that the message should be sent to.
func (d *downstreamSenderQueue) send(s sendInfo) {
	defer d.wg.Done()
	defer d.workers.Release()

	logging.Debug(d.logger).Log(logging.MessageKey(), "Sending message downstream...")

	response := s.handler.HandleMessage(s.msg)
	if response != nil {
		logging.Debug(d.logger).Log(logging.MessageKey(), "Downstream returned a response")
		d.sendFunc(response)
		return
	}

	logging.Debug(d.logger).Log(logging.MessageKey(), "Downstream Message Sent")
}
