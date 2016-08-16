package kratos

import (
	"bytes"
	"github.com/comcast/webpa-common/canonical"
	"github.com/comcast/webpa-common/wrp"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type ReadHandler interface {
	HandleMessage(msg interface{})
}

// what we expose to the user of kratos
type Client interface {
	Send(io.WriterTo) error
	Close() error
}

type websocketConnection interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

// internal data type for Client interface
type HandlerRegistry struct {
	HandlerKey string
	keyRegex   *regexp.Regexp
	Handler    ReadHandler
}

type client struct {
	deviceId        string
	userAgent       string
	deviceProtocols string
	handlers        []HandlerRegistry
	connection      websocketConnection
	headerInfo      *clientHeader
}

// used to track everything that we want to know about the client headers
type clientHeader struct {
	deviceName   string
	firmwareName string
	modelName    string
	manufacturer string
}

// factory used to generate a client
type ClientFactory struct {
	DeviceName     string
	FirmwareName   string
	ModelName      string
	Manufacturer   string
	DestinationUrl string
	Handlers       []HandlerRegistry
}

// used to open a channel for writing to servers
func (c *client) Send(message io.WriterTo) error {
	var buffer bytes.Buffer
	if _, err := message.WriteTo(&buffer); err != nil {
		return err
	}

	err := c.connection.WriteMessage(websocket.TextMessage, buffer.Bytes())
	if err != nil {
		return err
	}

	return nil
}

// will close the connection to the server
// TODO: determine if I should have this somehow destroy the client
// to prevent users from using it at all after call this
func (c *client) Close() error {
	if err := c.connection.Close(); err != nil {
		return err
	}

	return nil
}

// used to create a new kratos Client
func (f *ClientFactory) New() (Client, error) {
	inHeader := &clientHeader{
		deviceName:   f.DeviceName,
		firmwareName: f.FirmwareName,
		modelName:    f.ModelName,
		manufacturer: f.Manufacturer,
	}

	newConnection, err := createConnection(inHeader, f.DestinationUrl)
	if err != nil {
		return nil, err
	}

	newClient := &client{
		deviceId:        inHeader.deviceName,
		userAgent:       "WebPA-1.6(" + inHeader.firmwareName + ";" + inHeader.modelName + "/" + inHeader.manufacturer + ";)",
		deviceProtocols: "TODO-what-to-put-here",
		handlers:        f.Handlers,
		connection:      newConnection,
		headerInfo:      inHeader,
	}

	for i, _ := range newClient.handlers {
		newClient.handlers[i].keyRegex, err = regexp.Compile(newClient.handlers[i].HandlerKey)
		if err != nil {
			return nil, err
		}
	}

	go newClient.read()

	return newClient, nil
}

// going to be used to access the HandleMessage() function
func (c *client) read() error {
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

	return nil
}

// private func used to generate the client that we're looking to produce
func createConnection(headerInfo *clientHeader, destUrl string) (*websocket.Conn, error) {
	_, err := canonical.ParseId(headerInfo.deviceName)
	if err != nil {
		return nil, err
	}

	url, err := resolveURL(headerInfo.deviceName, destUrl)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	return connection, nil
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
