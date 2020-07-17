package kratos

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/goph/emperror"
	"github.com/xmidt-org/wrp-go/wrp"
)

// ErrNoDownstreamHandler provides an easy way for a consumer to do logic based
// on the error of no downstream handler being found.
type ErrNoDownstreamHandler interface {
	ErrNoDownstreamHandler()
}

// errNoDownstreamHandler is a sentinel error that implements ErrNoDownstreamHandler.
type errNoDownstreamHandler struct {
}

// ErrNoDownstreamHandler is a throwaway function to implement the interface,
// making it possible to assert the type of the errNoDownstreamHandler.
func (e errNoDownstreamHandler) ErrNoDownstreamHandler() {}

// Error ensures that errNoDownstreamHandler is an error.
func (e errNoDownstreamHandler) Error() string {
	return "no downstream handler found for provided destination"
}

// ErrInvalidHandler provides an easy way for a consumer to do logic based on
// the error of an invalid handler being found.
type ErrInvalidHandler interface {
	ErrInvalidHandler()
}

// errInvalidHandler is a sentinel error that implements ErrInvalidHandler.
type errInvalidHandler struct {
}

// ErrInvalidHandler is a throwaway function to implement the interface, making
// it possible to assert the type of the errInvalidHandler.
func (e errInvalidHandler) ErrInvalidHandler() {}

// Error ensures that errInvalidHandler is an error.
func (e errInvalidHandler) Error() string {
	return "handler cannot be nil"
}

// HandlerConfig is the values that a consumer can set that specify the handler
// to use for the regular expression.
type HandlerConfig struct {
	Regexp  string
	Handler DownstreamHandler
}

// HandlerGroup is an internal data type for Client interface
// that helps keep track of registered handler functions.
type HandlerGroup struct {
	keyRegex *regexp.Regexp
	handler  DownstreamHandler
}

// DownstreamHandler should be implemented by the user so that they
// may deal with received messages how they please.
type DownstreamHandler interface {
	HandleMessage(msg *wrp.Message) *wrp.Message
	Close()
}

// HandlerRegistry is an interface that handles adding, getting, and removing
// DownstreamHandlers.
type HandlerRegistry interface {
	Add(string, DownstreamHandler) error
	Remove(string)
	GetHandler(string) (DownstreamHandler, error)
	Close()
}

// handlerRegistry is our implementation for HandlerRegistry that can be used
// concurrently.
type handlerRegistry struct {
	store map[string]HandlerGroup
	lock  sync.RWMutex
}

// NewHandlerRegistry creates a handlerRegistry based on the initial handlers
// given.
func NewHandlerRegistry(config []HandlerConfig) (*handlerRegistry, error) {

	registry := handlerRegistry{
		store: make(map[string]HandlerGroup),
	}
	errs := errorList{}
	for _, c := range config {
		if c.Handler == nil {
			errs = append(errs, emperror.Wrap(errInvalidHandler{}, fmt.Sprintf("cannot add handler for regular expression [%v]", c.Regexp)))
			continue
		}
		r, err := regexp.Compile(c.Regexp)
		if err != nil {
			errs = append(errs, emperror.Wrap(err, fmt.Sprintf("failed to compile regular expression [%v]", c.Regexp)))
		} else {
			registry.store[c.Regexp] = HandlerGroup{keyRegex: r, handler: c.Handler}
		}
	}
	if len(errs) == 0 {
		return &registry, nil
	}
	return &registry, errs
}

// Add provides a way to add a new handler to a pre-existing handlerRegistry.
// If there is already a handler for the regular expression given, it is
// overwritten with the new handler.
func (h *handlerRegistry) Add(regexpName string, handler DownstreamHandler) error {
	if handler == nil {
		return errInvalidHandler{}
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	r, err := regexp.Compile(regexpName)
	if err != nil {
		return emperror.WrapWith(err, "failed to compile regular expression", "regexp", regexpName)
	}
	h.store[regexpName] = HandlerGroup{keyRegex: r, handler: handler}
	return nil
}

// Remove provides a way to remove an already existing handler in the
// handlerRegistry.
func (h *handlerRegistry) Remove(regexpName string) {
	h.lock.Lock()
	delete(h.store, regexpName)
	h.lock.Unlock()
}

// GetHandler gives the handler whose regular expression matches the
// destination given.  If there is no handler with a matching regular
// expression, an error is returned.
func (h *handlerRegistry) GetHandler(destination string) (DownstreamHandler, error) {
	h.lock.RLock()
	defer h.lock.RUnlock()
	for _, handler := range h.store {
		if handler.keyRegex.MatchString(destination) {
			return handler.handler, nil
		}
	}
	return nil, errNoDownstreamHandler{}
}

// Close calls the Close function on all the handlers in the handlerRegistry.
func (h *handlerRegistry) Close() {
	h.lock.Lock()
	defer h.lock.Unlock()
	for key, handler := range h.store {
		handler.handler.Close()
		delete(h.store, key)
	}
}
