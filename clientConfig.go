// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package kratos

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = time.Duration(10) * time.Second
)

var (
	errNilHandlePingMiss = errors.New("HandlePingMiss should not be nil")
)

// ClientConfig is the configuration to provide when making a new client.
type ClientConfig struct {
	DeviceName           string
	FirmwareName         string
	ModelName            string
	Manufacturer         string
	DestinationURL       string
	OutboundQueue        QueueConfig
	WRPEncoderQueue      QueueConfig
	WRPDecoderQueue      QueueConfig
	HandlerRegistryQueue QueueConfig
	HandleMsgQueue       QueueConfig
	Handlers             []HandlerConfig
	HandlePingMiss       HandlePingMiss
	ClientLogger         *zap.Logger
	PingConfig           PingConfig
}

// QueueConfig is used to configure all the queues used to make kratos asynchronous.
type QueueConfig struct {
	MaxWorkers int
	Size       int
}

type PingConfig struct {
	PingWait    time.Duration
	MaxPingMiss int
}

// NewClient is used to create a new kratos Client from a ClientConfig.
func NewClient(config ClientConfig) (Client, error) {
	if config.HandlePingMiss == nil {
		return nil, errNilHandlePingMiss
	}

	inHeader := &clientHeader{
		deviceName:   config.DeviceName,
		firmwareName: config.FirmwareName,
		modelName:    config.ModelName,
		manufacturer: config.Manufacturer,
	}

	newConnection, connectionURL, err := createConnection(inHeader, config.DestinationURL)

	if err != nil {
		return nil, err
	}

	pinged := make(chan string)
	newConnection.SetPingHandler(func(appData string) error {
		pinged <- appData
		err := newConnection.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(writeWait))
		return err
	})

	// at this point we know that the URL connection is legitimate, so we can do some string manipulation
	// with the knowledge that `:` will be found in the string twice
	connectionURL = connectionURL[len("ws://"):strings.LastIndex(connectionURL, ":")]

	var logger *zap.Logger
	if config.ClientLogger != nil {
		logger = config.ClientLogger
	} else {
		logger = sallust.Default()
	}
	if config.PingConfig.MaxPingMiss <= 0 {
		config.PingConfig.MaxPingMiss = 1
	}
	if config.PingConfig.PingWait == 0 {
		config.PingConfig.PingWait = time.Minute
	}

	sender := NewSender(newConnection, config.OutboundQueue.MaxWorkers, config.OutboundQueue.Size, logger)
	encoder := NewEncoderSender(sender, config.WRPEncoderQueue.MaxWorkers, config.WRPEncoderQueue.Size, logger)

	newClient := &client{
		deviceID:        inHeader.deviceName,
		userAgent:       "WebPA-1.6(" + inHeader.firmwareName + ";" + inHeader.modelName + "/" + inHeader.manufacturer + ";)",
		deviceProtocols: "TODO-what-to-put-here",
		hostname:        connectionURL,
		handlePingMiss:  config.HandlePingMiss,
		encoderSender:   encoder,
		connection:      newConnection,
		headerInfo:      inHeader,
		done:            make(chan struct{}, 1),
		logger:          logger,
		pingConfig:      config.PingConfig,
	}

	newClient.registry, err = NewHandlerRegistry(config.Handlers)
	if err != nil {
		logger.Warn("failed to initialize all handlers for registry", zap.Error(err))
	}

	downstreamSender := NewDownstreamSender(newClient.Send, config.HandleMsgQueue.MaxWorkers, config.HandleMsgQueue.Size, logger)
	registryHandler := NewRegistryHandler(newClient.Send, newClient.registry, downstreamSender, config.HandlerRegistryQueue.MaxWorkers, config.HandlerRegistryQueue.Size, newClient.deviceID, logger)
	decoder := NewDecoderSender(registryHandler, config.WRPDecoderQueue.MaxWorkers, config.WRPDecoderQueue.Size, logger)
	newClient.decoderSender = decoder

	pingTimer := time.NewTimer(newClient.pingConfig.PingWait)

	newClient.wg.Add(2)
	go newClient.checkPing(pingTimer, pinged)

	go newClient.read()

	return newClient, nil
}

// private func used to generate the client that we're looking to produce
func createConnection(headerInfo *clientHeader, httpURL string) (connection *websocket.Conn, wsURL string, err error) {
	_, err = wrp.ParseDeviceID(headerInfo.deviceName)

	if err != nil {
		return nil, "", err
	}

	// make a header and put some data in that (including MAC address)
	// TODO: find special function for user agent
	headers := make(http.Header)
	headers.Add("X-Webpa-Device-Name", headerInfo.deviceName)
	headers.Add("X-Webpa-Firmware-Name", headerInfo.firmwareName)
	headers.Add("X-Webpa-Model-Name", headerInfo.modelName)
	headers.Add("X-Webpa-Manufacturer", headerInfo.manufacturer)

	// make sure destUrl's protocol is websocket (ws)
	wsURL = strings.Replace(httpURL, "http", "ws", 1)

	// creates a new client connection given the URL string
	connection, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
	for errors.Is(err, websocket.ErrBadHandshake) && resp != nil && resp.StatusCode == http.StatusTemporaryRedirect {
		// Get url to which we are redirected and reconfigure it
		wsURL = strings.Replace(resp.Header.Get("Location"), "http", "ws", 1)

		connection, resp, err = websocket.DefaultDialer.Dial(wsURL, headers)
	}

	defer resp.Body.Close()

	if err != nil {
		if resp != nil {
			err = createHTTPError(resp, err)
		}
		return nil, "", err
	}

	return connection, wsURL, nil
}
