package kratos

import (
	"bytes"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/wrp"
	"github.com/gorilla/websocket"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = time.Duration(10) * time.Second

	// Time allowed to wait in between pings
	pingWait = time.Duration(60) * time.Second
)

// ClientFactory is used to generate a client by calling new on this type
type ClientFactory struct {
	DeviceName     string
	FirmwareName   string
	ModelName      string
	Manufacturer   string
	DestinationUrl string
	Handlers       []HandlerRegistry
	HandlePingMiss HandlePingMiss
	ClientLogger   logging.Logger
}

// New is used to create a new kratos Client from a ClientFactory
func (f *ClientFactory) New() (Client, error) {
	inHeader := &clientHeader{
		deviceName:   f.DeviceName,
		firmwareName: f.FirmwareName,
		modelName:    f.ModelName,
		manufacturer: f.Manufacturer,
	}

	newConnection, connectionURL, err := createConnection(inHeader, f.DestinationUrl)
	if err != nil {
		return nil, err
	}

	pinged := make(chan string)
	newConnection.SetPingHandler(func(appData string) error {
		pinged <- appData
		err := newConnection.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(writeWait))
		return err
	})

	// at this point we know that the URL connection is legitimate, so we can do some string manipulation
	// with the knowledge that `:` will be found in the string twice
	connectionURL = connectionURL[len("ws://"):strings.LastIndex(connectionURL, ":")]

	newClient := &client{
		deviceId:        inHeader.deviceName,
		userAgent:       "WebPA-1.6(" + inHeader.firmwareName + ";" + inHeader.modelName + "/" + inHeader.manufacturer + ";)",
		deviceProtocols: "TODO-what-to-put-here",
		hostname:        connectionURL,
		handlers:        f.Handlers,
		connection:      newConnection,
		headerInfo:      inHeader,
	}

	pingMissHandler := PingMissHandler{
		handlePingMiss: f.HandlePingMiss,
	}

	if f.ClientLogger != nil {
		newClient.Logger = f.ClientLogger
		pingMissHandler.Logger = f.ClientLogger
	} else {
		newClient.Logger = &logging.LoggerWriter{ioutil.Discard}
		pingMissHandler.Logger = &logging.LoggerWriter{ioutil.Discard}
	}

	for i := range newClient.handlers {
		newClient.handlers[i].keyRegex, err = regexp.Compile(newClient.handlers[i].HandlerKey)
		if err != nil {
			return nil, err
		}
	}

	pingTimer := time.NewTimer(pingWait)

	go pingMissHandler.checkPing(pingTimer, pinged, newClient)
	go newClient.read()

	return newClient, nil
}

// function called when we run into situations where we're not getting anymore pings
// the implementation of this function needs to be handled by the user of kratos
type HandlePingMiss func(inClient Client) error

type PingMissHandler struct {
	handlePingMiss HandlePingMiss
	logging.Logger
}

func (pmh *PingMissHandler) checkPing(inTimer *time.Timer, pinged <-chan string, inClient Client) {
	for {
		select {
		case <-inTimer.C:
			pmh.Info("Ping miss!")
			err := pmh.handlePingMiss(inClient)
			if err != nil {
				pmh.Info("Error handling ping miss:", err)
			}
		case <-pinged:
			if !inTimer.Stop() {
				<-inTimer.C
			}
			inTimer.Reset(pingWait)
		}
	}
}

// Client is what function calls we expose to the user of kratos
type Client interface {
	Hostname() string
	Send(io.WriterTo) error
	Close() error
}

type websocketConnection interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

// ReadHandler should be implemented by the user so that they
// may deal with received messages how they please
type ReadHandler interface {
	HandleMessage(msg interface{})
}

// HandlerRegistry is an internal data type for Client interface
// that helps keep track of registered handler functions
type HandlerRegistry struct {
	HandlerKey string
	keyRegex   *regexp.Regexp
	Handler    ReadHandler
}

type client struct {
	deviceId        string
	userAgent       string
	deviceProtocols string
	hostname        string
	handlers        []HandlerRegistry
	connection      websocketConnection
	headerInfo      *clientHeader
	logging.Logger
}

// used to track everything that we want to know about the client headers
type clientHeader struct {
	deviceName   string
	firmwareName string
	modelName    string
	manufacturer string
}

func (c *client) Hostname() string {
	return c.hostname
}

// used to open a channel for writing to servers
func (c *client) Send(message io.WriterTo) error {
	c.Info("Sending message...")

	var buffer bytes.Buffer
	if _, err := message.WriteTo(&buffer); err != nil {
		return err
	}

	err := c.connection.WriteMessage(websocket.BinaryMessage, buffer.Bytes())
	if err != nil {
		return err
	}

	return nil
}

// will close the connection to the server
// TODO: determine if I should have this somehow destroy the client
// to prevent users from using it at all after call this
func (c *client) Close() error {
	c.Info("Closing client...")

	if err := c.connection.Close(); err != nil {
		return err
	}

	return nil
}

// going to be used to access the HandleMessage() function
func (c *client) read() error {
	c.Info("Reading message...")

	for {
		_, serverMessage, err := c.connection.ReadMessage()
		if err != nil {
			return err
		}

		// decode the message so we can read it
		data, err := wrp.Decode(serverMessage)
		if err != nil {
			return err
		}

		if _, ok := data.(wrp.WrpMsg); ok {
			for i := 0; i < len(c.handlers); i++ {
				if c.handlers[i].keyRegex.MatchString(data.(wrp.WrpMsg).Destination()) {
					c.handlers[i].Handler.HandleMessage(data)
				}
			}
		}
	}
}

// private func used to generate the client that we're looking to produce
func createConnection(headerInfo *clientHeader, destUrl string) (*websocket.Conn, string, error) {
	url, err := resolveURL(headerInfo.deviceName, destUrl)
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

	// creates a new client connection given the URL string
	connection, _, err := websocket.DefaultDialer.Dial(url.String(), headers)
	if err != nil {
		return nil, "", err
	}

	return connection, url.String(), nil
}

// private func used to resolve the URL that we're given in case of redirects
func resolveURL(deviceId string, fabricUrl string) (*url.URL, error) {
	// declare client as a pointer to a new http client struct
	client := &http.Client{}

	// get a Request suitable for use with Client.Do
	req, err := http.NewRequest("GET", fabricUrl, nil)
	if err != nil {
		return nil, err
	}

	// turn off keep alive
	req.Close = true

	// add the device name and MAC address to the http header
	// TODO: do we need to populate the header with everything here, too?
	req.Header.Add("X-Webpa-Device-Name", deviceId)

	// send an http request and receive a response from the server
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// replace the `https` in the URL with `wss`
	actualURL, err := url.Parse(strings.Replace(resp.Request.URL.String(), "http", "ws", 1))
	if err != nil {
		return nil, err
	}

	return actualURL, nil
}
