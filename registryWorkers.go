package kratos

import (
	"net/http"
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/goph/emperror"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/wrp"
)

type registryHandler interface {
	GetHandlerThenSend(*wrp.Message)
	Close()
}

type registryQueue struct {
	incoming         chan *wrp.Message
	registry         HandlerRegistry
	sendFunc         sendWRPFunc
	downstreamSender downstreamSender
	deviceID         string
	workers          semaphore.Interface
	wg               sync.WaitGroup
	logger           log.Logger
}

func NewRegistryHandler(senderFunc sendWRPFunc, registry HandlerRegistry, downstreamSender downstreamSender, maxWorkers int, queueSize int, deviceID string, logger log.Logger) *registryQueue {
	size := queueSize
	if size < minQueueSize {
		size = minQueueSize
	}
	numWorkers := maxWorkers
	if numWorkers < minWorkers {
		numWorkers = minWorkers
	}
	r := registryQueue{
		incoming:         make(chan *wrp.Message, size),
		registry:         registry,
		sendFunc:         senderFunc,
		downstreamSender: downstreamSender,
		deviceID:         deviceID,
		workers:          semaphore.New(numWorkers),
		logger:           logger,
	}
	r.wg.Add(1)
	go r.startGettingHandlers()
	return &r
}

func (r *registryQueue) GetHandlerThenSend(msg *wrp.Message) {
	r.incoming <- msg
}

func (r *registryQueue) Close() {
	close(r.incoming)
	r.wg.Wait()
	r.registry.Close()
	r.downstreamSender.Close()
}

func (r *registryQueue) startGettingHandlers() {
	defer r.wg.Done()
	for i := range r.incoming {
		r.workers.Acquire()
		r.wg.Add(1)
		go r.getHandler(i)
	}
}

func (r *registryQueue) getHandler(msg *wrp.Message) {
	defer r.wg.Done()
	defer r.workers.Release()

	logging.Debug(r.logger).Log(logging.MessageKey(), "Getting handler...")

	handler, err := r.registry.GetHandler(msg.Destination)
	if _, ok := err.(ErrNoDownstreamHandler); ok {
		// If no valid handlers for the destination, create a new simple RequestResponse wrp with http Status Code of Service Unavailable
		response := CreateErrorWRP(msg.TransactionUUID, msg.Source, r.deviceID, http.StatusServiceUnavailable, emperror.Wrap(err, "unable to get handler"))
		logging.Error(r.logger, emperror.Context(err)...).
			Log(logging.MessageKey(), "Failed to get handler", logging.ErrorKey(), err.Error())
		r.sendFunc(response)
		return
	}
	if err != nil {
		// for now, do the same as if there is no downstream handler.
		response := CreateErrorWRP(msg.TransactionUUID, msg.Source, r.deviceID, http.StatusServiceUnavailable, emperror.Wrap(err, "unable to get handler"))
		logging.Error(r.logger, emperror.Context(err)...).
			Log(logging.MessageKey(), "Failed to get handler", logging.ErrorKey(), err.Error())
		r.sendFunc(response)
		return
	}

	r.downstreamSender.Send(handler, msg)

	logging.Debug(r.logger).Log(logging.MessageKey(), "Sent message to handler")

}
