package kratos

import (
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/wrp"
)

type downstreamSender interface {
	Send(DownstreamHandler, *wrp.Message)
	Close()
}

type sendInfo struct {
	handler DownstreamHandler
	msg     *wrp.Message
}

type downstreamSenderQueue struct {
	incoming chan sendInfo
	sendFunc sendWRPFunc
	workers  semaphore.Interface
	wg       sync.WaitGroup
	logger   log.Logger
}

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

func (d *downstreamSenderQueue) Send(handler DownstreamHandler, msg *wrp.Message) {
	d.incoming <- sendInfo{handler: handler, msg: msg}
}

func (d *downstreamSenderQueue) Close() {
	close(d.incoming)
	d.wg.Wait()
}

func (d *downstreamSenderQueue) startSending() {
	defer d.wg.Done()
	for i := range d.incoming {
		d.workers.Acquire()
		d.wg.Add(1)
		go d.send(i)
	}
}

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
