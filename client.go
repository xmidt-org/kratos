package kratos

import (
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/goph/emperror"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/wrp-go/wrp"
)

// Client is what function calls we expose to the user of kratos
type Client interface {
	Hostname() string
	HandlerRegistry() HandlerRegistry
	Send(message *wrp.Message)
	Close() error
}

type sendWRPFunc func(*wrp.Message)

type client struct {
	deviceID        string
	userAgent       string
	deviceProtocols string
	hostname        string
	registry        HandlerRegistry
	handlePingMiss  HandlePingMiss
	encoderSender   encoderSender
	decoderSender   decoderSender
	connection      websocketConnection
	headerInfo      *clientHeader
	logger          log.Logger
	done            chan struct{}
	wg              sync.WaitGroup
}

// used to track everything that we want to know about the client headers
type clientHeader struct {
	deviceName   string
	firmwareName string
	modelName    string
	manufacturer string
}

type websocketConnection interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

func (c *client) Hostname() string {
	return c.hostname
}

func (c *client) HandlerRegistry() HandlerRegistry {
	return c.registry
}

// used to open a channel for writing to XMiDT
func (c *client) Send(message *wrp.Message) {
	c.encoderSender.EncodeAndSend(message)
}

// will close the connection to the server
func (c *client) Close() error {
	logging.Info(c.logger).Log(logging.MessageKey(), "Closing client...")
	close(c.done)
	c.wg.Wait()
	c.decoderSender.Close()
	c.encoderSender.Close()
	err := c.connection.Close()
	if err != nil {
		return emperror.Wrap(err, "Failed to close connection")
	}
	logging.Info(c.logger).Log(logging.MessageKey(), "Client Closed")
	return nil
}

// going to be used to access the HandleMessage() function
func (c *client) read() {
	defer c.wg.Done()
	logging.Info(c.logger).Log(logging.MessageKey(), "Watching socket for messages.")

	for {
		select {
		case <-c.done:
			logging.Info(c.logger).Log(logging.MessageKey(), "Stopped reading from socket.")
			return
		default:
			logging.Debug(c.logger).Log(logging.MessageKey(), "Reading message...")

			_, serverMessage, err := c.connection.ReadMessage()
			if err != nil {
				logging.Error(c.logger, emperror.Context(err)...).
					Log(logging.MessageKey(), "Failed to read message", logging.ErrorKey(), err.Error())
				continue
			}
			c.decoderSender.DecodeAndSend(serverMessage)

			logging.Debug(c.logger).Log(logging.MessageKey(), "Message sent to be decoded")
		}
	}

	return
}
