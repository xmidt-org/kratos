package kratos

import (
	"sync"

	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

// Client is what function calls we expose to the user of kratos
type Client interface {
	Hostname() string
	HandlerRegistry() HandlerRegistry
	Send(message *wrp.Message)
	Close() error
}

// sendWRPFunc is the function for sending a message downstream.
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
	logger          *zap.Logger
	done            chan struct{}
	wg              sync.WaitGroup
	pingConfig      PingConfig
	once            sync.Once
}

// used to track everything that we want to know about the client headers
type clientHeader struct {
	deviceName   string
	firmwareName string
	modelName    string
	manufacturer string
}

// websocketConnection maintains the websocket connection upstream (to XMiDT).
type websocketConnection interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

// Hostname provides the client's hostname.
func (c *client) Hostname() string {
	return c.hostname
}

// HandlerRegistry returns the HandlerRegistry that the client maintains.
func (c *client) HandlerRegistry() HandlerRegistry {
	return c.registry
}

// Send is used to open a channel for writing to XMiDT
func (c *client) Send(message *wrp.Message) {
	c.encoderSender.EncodeAndSend(message)
}

// Close closes connections downstream and the socket upstream.
func (c *client) Close() error {
	var connectionErr error
	c.once.Do(func() {
		c.logger.Info("Closing client...")
		close(c.done)
		c.wg.Wait()
		c.decoderSender.Close()
		c.encoderSender.Close()
		connectionErr = c.connection.Close()
		c.connection = nil
		// TODO: if this fails, can we really do anything. Is there potential for leaks?
		// if err != nil {
		// 	return emperror.Wrap(err, "Failed to close connection")
		// }
		c.logger.Info("Client Closed")
	})
	return connectionErr
}

// going to be used to access the HandleMessage() function
func (c *client) read() {
	defer c.wg.Done()
	c.logger.Info("Watching socket for messages.")

	for {
		select {
		case <-c.done:
			c.logger.Info("Stopped reading from socket.")
			return
		default:
			c.logger.Info("Reading message...")

			_, serverMessage, err := c.connection.ReadMessage()
			if err != nil {
				c.logger.Error("Failed to read message. Exiting out of read loop.", zap.Error(err))
				return
			}
			c.decoderSender.DecodeAndSend(serverMessage)

			c.logger.Debug("Message sent to be decoded")
		}
	}
}
