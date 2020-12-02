package kratos

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/go-kit/kit/log"
	"github.com/goph/emperror"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/v3"
)

// registryHandler is a way to send the wrp message to the correct handler.
type registryHandler interface {
	GetHandlerThenSend(*wrp.Message)
	Close()
}

// registryQueue provides a way to use the HandlerRegistry in an asynchronous
// fashion.  The registryQueue gets the handler for the wrp message from the
// handler registry, and then calls on the downstreamSender to send the message
// to that handler.
type registryQueue struct {
	incoming         chan *wrp.Message
	registry         HandlerRegistry
	sendFunc         sendWRPFunc
	downstreamSender downstreamSender
	deviceID         string
	workers          semaphore.Interface
	wg               sync.WaitGroup
	logger           log.Logger
	once             sync.Once
	closed           atomic.Value
}

// NewRegistryHandler returns a registryHandler, which sends wrp messages to
// the correct handler in an asynchronous fashion.
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

// GetHandlerThenSend adds the message to the queue, so it can be handled when
// there are appropriate resources.
func (r *registryQueue) GetHandlerThenSend(msg *wrp.Message) {
	switch r.closed.Load() {
	case true:
		fmt.Println("d")
	default:
		r.incoming <- msg
	}
}

// Close is a graceful shutdown of the registryQueue: first getting handlers and
// sending the currently held events, then closing the downstreamSender.
func (r *registryQueue) Close() {
	r.once.Do(func() {
		r.closed.Store(true)
		close(r.incoming)
		r.wg.Wait()
		r.registry.Close()
		r.downstreamSender.Close()
	})
}

// startGettingHandlers is called when the registryQueue starts, enabling the
// registryQueue to read from its queue, get the appropriate handler for the
// given message, and send it using the downstreamSender.
func (r *registryQueue) startGettingHandlers() {
	defer r.wg.Done()
	for i := range r.incoming {
		r.workers.Acquire()
		r.wg.Add(1)
		go r.getHandler(i)
	}
}

// getHandler provides a way to get the handler from the registry and then send
// the message.
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
