package kratos

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/goph/emperror"
	"github.com/xmidt-org/wrp-go/wrp"
)

type ErrNoDownstreamHandler interface {
	ErrNoDownstreamHandler()
}

type errNoDownstreamHandler struct {
}

func (e errNoDownstreamHandler) ErrNoDownstreamHandler() {}
func (e errNoDownstreamHandler) Error() string {
	return "no downstream handler found for provided destination"
}

type ErrInvalidHandler interface {
	ErrInvalidHandler()
}

type errInvalidHandler struct {
}

func (e errInvalidHandler) ErrInvalidHandler() {}
func (e errInvalidHandler) Error() string {
	return "handler cannot be nil"
}

// HandlerConfig is the values that a consumer can set in order to
type HandlerConfig struct {
	Regexp  string
	Handler DownstreamHandler
}

// HandlerGroup is an internal data type for Client interface
// that helps keep track of registered handler functions
type HandlerGroup struct {
	keyRegex *regexp.Regexp
	handler  DownstreamHandler
}

// DownstreamHandler should be implemented by the user so that they
// may deal with received messages how they please
type DownstreamHandler interface {
	HandleMessage(msg *wrp.Message) *wrp.Message
	Close()
}

type HandlerRegistry interface {
	Add(string, DownstreamHandler) error
	Remove(string)
	GetHandler(string) (DownstreamHandler, error)
	Close()
}

type handlerRegistry struct {
	store map[string]HandlerGroup
	lock  sync.RWMutex
}

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

func (h *handlerRegistry) Remove(regexpName string) {
	h.lock.Lock()
	delete(h.store, regexpName)
	h.lock.Unlock()
}

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

func (h *handlerRegistry) Close() {
	h.lock.Lock()
	defer h.lock.Unlock()
	for key, handler := range h.store {
		handler.handler.Close()
		delete(h.store, key)
	}
}
