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

type decoderSender interface {
	DecodeAndSend([]byte)
	Close()
}

type decoderQueue struct {
	incoming chan []byte
	sender   registryHandler
	workers  semaphore.Interface
	wg       sync.WaitGroup
	logger   log.Logger
}

func NewDecoder(sender registryHandler, maxWorkers int, queueSize int, logger log.Logger) *decoderQueue {
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

func (d *decoderQueue) DecodeAndSend(msg []byte) {
	d.incoming <- msg
}

func (d *decoderQueue) Close() {
	close(d.incoming)
	d.wg.Wait()
	d.sender.Close()
}

func (d *decoderQueue) startParsing() {
	defer d.wg.Done()
	for i := range d.incoming {
		d.workers.Acquire()
		d.wg.Add(1)
		go d.parse(i)
	}
}

func (d *decoderQueue) parse(incoming []byte) {
	defer d.wg.Done()
	defer d.workers.Release()
	msg := wrp.Message{}

	logging.Debug(d.logger).Log(logging.MessageKey(), "Decoding message...")

	err := wrp.NewDecoderBytes(incoming, wrp.Msgpack).Decode(&msg)
	if err != nil {
		logging.Error(d.logger, emperror.Context(err)...).
			Log(logging.MessageKey(), "Failed to decode message into wrp", logging.ErrorKey(), err.Error())
		return
	}

	logging.Debug(d.logger).Log(logging.MessageKey(), "Message Decoded")
}
