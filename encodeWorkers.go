package kratos

import (
	"bytes"
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/goph/emperror"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/wrp"
)

type encoderSender interface {
	EncodeAndSend(*wrp.Message)
	Close()
}

type encoderQueue struct {
	incoming chan *wrp.Message
	sender   outboundSender
	workers  semaphore.Interface
	wg       sync.WaitGroup
	logger   log.Logger
}

func NewEncoder(sender outboundSender, maxWorkers int, queueSize int, logger log.Logger) *encoderQueue {
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

func (e *encoderQueue) EncodeAndSend(msg *wrp.Message) {
	e.incoming <- msg
}

func (e *encoderQueue) Close() {
	close(e.incoming)
	e.wg.Wait()
	e.sender.Close()
}

func (e *encoderQueue) startParsing() {
	defer e.wg.Done()
	for i := range e.incoming {
		e.workers.Acquire()
		e.wg.Add(1)
		go e.parse(i)
	}
}

func (e *encoderQueue) parse(incoming *wrp.Message) {
	defer e.wg.Done()
	defer e.workers.Release()
	var buffer bytes.Buffer

	logging.Debug(e.logger).Log(logging.MessageKey(), "Encoding message...")

	err := wrp.NewEncoder(&buffer, wrp.Msgpack).Encode(incoming)
	if err != nil {
		logging.Error(e.logger, emperror.Context(err)...).
			Log(logging.MessageKey(), "Failed to encode message",
				logging.ErrorKey(), err.Error(),
				"message", incoming)
		return
	}
	e.sender.Send(buffer.Bytes())

	logging.Debug(e.logger).Log(logging.MessageKey(), "Message Encoded")
}
