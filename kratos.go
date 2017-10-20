package kratos

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/wrp"
	"github.com/go-kit/kit/log"
	"github.com/gorilla/websocket"
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
	ClientLogger   log.Logger
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
		err := newConnection.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(writeWait))
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

	myPingMissHandler := pingMissHandler{
		handlePingMiss: f.HandlePingMiss,
	}

	if f.ClientLogger != nil {
		newClient.Logger = f.ClientLogger
		myPingMissHandler.Logger = f.ClientLogger
	} else {
		newClient.Logger = logging.DefaultLogger()
		myPingMissHandler.Logger = logging.DefaultLogger()
	}

	for i := range newClient.handlers {
		newClient.handlers[i].keyRegex, err = regexp.Compile(newClient.handlers[i].HandlerKey)
		if err != nil {
			return nil, err
		}
	}

	pingTimer := time.NewTimer(pingWait)

	go myPingMissHandler.checkPing(pingTimer, pinged, newClient)
	go newClient.read()

	return newClient, nil
}

// function called when we run into situations where we're not getting anymore pings
// the implementation of this function needs to be handled by the user of kratos
type HandlePingMiss func() error

type pingMissHandler struct {
	handlePingMiss HandlePingMiss
	log.Logger
}

func (pmh *pingMissHandler) checkPing(inTimer *time.Timer, pinged <-chan string, inClient Client) {
	defer inClient.Close()
	pingMiss := false

	for !pingMiss {
		select {
		case <-inTimer.C:
			logging.Info(pmh).Log(logging.MessageKey(), "Ping miss, calling handler and closing client!")
			pingMiss = true
			err := pmh.handlePingMiss()
			if err != nil {
				logging.Info(pmh).Log(logging.MessageKey(), "Error handling ping miss:", logging.ErrorKey(), err)
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
	Send(message interface{}) error
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
	log.Logger
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
func (c *client) Send(message interface{}) (err error) {
	logging.Info(c).Log(logging.MessageKey(), "Sending message...")

	var buffer bytes.Buffer

	if err = wrp.NewEncoder(&buffer, wrp.Msgpack).Encode(message); err == nil {
		err = c.connection.WriteMessage(websocket.BinaryMessage, buffer.Bytes())
	}
	return
}

// will close the connection to the server
func (c *client) Close() (err error) {
	logging.Info(c).Log("Closing client...")

	err = c.connection.Close()
	return
}

// going to be used to access the HandleMessage() function
func (c *client) read() (err error) {
	logging.Info(c).Log("Reading message...")

	for {
		var serverMessage []byte
		_, serverMessage, err = c.connection.ReadMessage()
		if err != nil {
			return
		}

		// decode the message so we can read it
		wrpData := wrp.Message{}
		err = wrp.NewDecoderBytes(serverMessage, wrp.Msgpack).Decode(&wrpData)

		if err != nil {
			return
		}

		for i := 0; i < len(c.handlers); i++ {
			if c.handlers[i].keyRegex.MatchString(wrpData.Destination) {
				c.handlers[i].Handler.HandleMessage(wrpData)
			}
		}
	}

	return
}

// private func used to generate the client that we're looking to produce
func createConnection(headerInfo *clientHeader, destUrl string) (*websocket.Conn, string, error) {
	_, err := device.ParseID(headerInfo.deviceName)
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

	//make sure destUrl's protocol is websocket (ws)
	destUrl = strings.Replace(destUrl, "http", "ws", 1)

	// creates a new client connection given the URL string
	connection, resp, err := websocket.DefaultDialer.Dial(destUrl, headers)

	if err == websocket.ErrBadHandshake && resp.StatusCode == http.StatusTemporaryRedirect {
		//Get url to which we are redirected and reconfigure it
		destUrl = strings.Replace(resp.Header.Get("Location"), "http", "ws", 1)

		connection, _, err = websocket.DefaultDialer.Dial(destUrl, headers)
	}

	if err != nil {
		return nil, "", err
	}

	return connection, destUrl, nil
}
