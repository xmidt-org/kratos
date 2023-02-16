package kratos

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/wrp-go/v3"
)

const (
	socketBufferSize = 1024
)

var (
	simpleQueueConfig = QueueConfig{
		MaxWorkers: 10,
		Size:       10,
	}
	// handlerCalled is true and used for synchronization, true meaning that
	// we don't need to worry about synchronization (mainly for calls to TestRead)
	clientConfig = ClientConfig{
		DeviceName:           "mac:ffffff112233",
		FirmwareName:         "TG1682_2.1p7s1_PROD_sey",
		ModelName:            "TG1682G",
		Manufacturer:         "ARRIS Group, Inc.",
		DestinationURL:       "",
		OutboundQueue:        simpleQueueConfig,
		WRPEncoderQueue:      simpleQueueConfig,
		WRPDecoderQueue:      simpleQueueConfig,
		HandlerRegistryQueue: simpleQueueConfig,
		HandleMsgQueue:       simpleQueueConfig,
		Handlers: []HandlerConfig{
			{
				Regexp: "/foo",
				Handler: &myReadHandler{
					helloMsg:      "Hello.",
					goodbyeMsg:    "I am Kratos.",
					handlerCalled: true,
				},
			},
			{
				Regexp: "/bar",
				Handler: &myReadHandler{
					helloMsg:      "Whaddup.",
					goodbyeMsg:    "It's dat boi Kratos.",
					handlerCalled: true,
				},
			},
			{
				Regexp: "/.*",
				Handler: &myReadHandler{
					helloMsg:      "Hey.",
					goodbyeMsg:    "Have you met Kratos?",
					handlerCalled: true,
				},
			},
		},
		HandlePingMiss: func() error {
			fmt.Println("hi")
			return nil
		},
		ClientLogger: nil,
		PingConfig:   PingConfig{},
	}

	testServer *httptest.Server

	goodMsg []byte

	ErrFoo = errors.New("this was supposed to happen")

	upgrader = &websocket.Upgrader{
		ReadBufferSize:  socketBufferSize,
		WriteBufferSize: socketBufferSize,
	}

	mainWG sync.WaitGroup
)

/****************** BEGIN MOCK DECLARATIONS ***********************/
type mockClient struct {
	mock.Mock
}

func (m *mockClient) Hostname() string {
	arguments := m.Called()
	return arguments.String(0)
}

func (m *mockClient) Send(message interface{}) error {
	arguments := m.Called(message)
	return arguments.Error(0)
}

func (m *mockClient) Close() error {
	arguments := m.Called()
	return arguments.Error(0)
}

type mockConnection struct {
	mock.Mock
}

func (m *mockConnection) WriteMessage(messageType int, data []byte) error {
	arguments := m.Called(messageType, data)
	return arguments.Error(0)
}

func (m *mockConnection) ReadMessage() (messageType int, p []byte, err error) {
	arguments := m.Called()
	return arguments.Int(0), arguments.Get(1).([]byte), arguments.Error(2)
}

func (m *mockConnection) Close() error {
	arguments := m.Called()
	return arguments.Error(0)
}

/******************* END MOCK DECLARATIONS ************************/

type myReadHandler struct {
	helloMsg      string
	goodbyeMsg    string
	handlerCalled bool
}

func (m *myReadHandler) HandleMessage(msg *wrp.Message) *wrp.Message {
	if !m.handlerCalled {
		mainWG.Done()
		m.handlerCalled = true
	}
	return msg
}

func (m *myReadHandler) Close() {
}

func TestMain(m *testing.M) {
	testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader.Upgrade(w, r, nil)
	}))
	defer testServer.Close()

	clientConfig.DestinationURL = testServer.URL

	wrpMsg := wrp.SimpleRequestResponse{
		Source:          "mac:ffffff112233/emu",
		Destination:     "/bar",
		TransactionUUID: "emu:unique",
		Payload:         []byte("the payload has reached the checkpoint"),
	}

	var buf bytes.Buffer

	wrp.NewEncoder(&buf, wrp.Msgpack).Encode(wrpMsg)

	goodMsg = buf.Bytes()

	os.Exit(m.Run())
}

func TestErrorCreation(t *testing.T) {
	assert := assert.New(t)
	code := StatusDeviceDisconnected
	msg := "ErrorDeviceBusy"

	brokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		fmt.Fprintf(
			w,
			`{"code": %d, "message": "%s"}`,
			code,
			msg,
		)
	}))
	defer brokenServer.Close()

	clientConfig.DestinationURL = brokenServer.URL
	_, err := NewClient(clientConfig)

	clientConfig.DestinationURL = testServer.URL

	assert.NotNil(err)
	expected := fmt.Sprintf("message: %s with error: %s", Message{code, msg}, websocket.ErrBadHandshake)
	assert.Equal(expected, err.Error())
}

func TestNew(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	testClient, err := NewClient(clientConfig)
	require.NoError(err)

	assert.Equal("127.0.0.1", testClient.Hostname())
	assert.Nil(err)
}

func TestNewBrokenMAC(t *testing.T) {
	assert := assert.New(t)
	goodMac := clientConfig.DeviceName
	clientConfig.DeviceName = "broken:mac"
	_, err := NewClient(clientConfig)

	clientConfig.DeviceName = goodMac
	assert.NotNil(err)
}

func TestNewBrokenURL(t *testing.T) {
	assert := assert.New(t)
	goodURL := clientConfig.DestinationURL
	clientConfig.DestinationURL = "broken.url"
	_, err := NewClient(clientConfig)

	clientConfig.DestinationURL = goodURL
	assert.NotNil(err)
}

func TestBadHandshake(t *testing.T) {
	assert := assert.New(t)

	brokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// do nothing so the websocket receives a bad handshake
	}))
	defer brokenServer.Close()

	clientConfig.DestinationURL = brokenServer.URL
	_, err := NewClient(clientConfig)

	clientConfig.DestinationURL = testServer.URL

	assert.NotNil(err)
}

// test the happy-path of sending a message through a websocket
func TestSend(t *testing.T) {
	fakeConn := &mockConnection{}
	fakeConn.On("WriteMessage", websocket.BinaryMessage, mock.AnythingOfType("[]uint8")).Return(nil).Once()

	myMessage := wrp.Message{
		Source:      "mac:ffffff112233/emu",
		Destination: "event:device-status/bla/bla",
		Payload:     []byte("the payload has reached the checkpoint"),
	}
	logger := sallust.Default()

	sender := NewSender(fakeConn, 1, 1, logger)
	encoder := NewEncoderSender(sender, 1, 1, logger)
	testClient := &client{
		encoderSender: encoder,
		connection:    fakeConn,
		logger:        logger,
	}

	testClient.Send(&myMessage)
	// TODO: remove sleep function
	time.Sleep(time.Second)
	fakeConn.AssertExpectations(t)
}

// test what happens when a websocket fails to write a message
func TestSendBrokenWriteMessage(t *testing.T) {
	fakeConn := &mockConnection{}
	fakeConn.On("WriteMessage", websocket.BinaryMessage, mock.AnythingOfType("[]uint8")).Return(ErrFoo).Once()

	logger := sallust.Default()

	sender := NewSender(fakeConn, 1, 1, logger)
	encoder := NewEncoderSender(sender, 1, 1, logger)
	testClient := &client{
		encoderSender: encoder,
		connection:    fakeConn,
		logger:        logger,
	}

	testClient.Send(nil)
	// TODO: remove sleep function
	time.Sleep(time.Second)
	fakeConn.AssertExpectations(t)
}

// test the happy path of closing a websocket once we're finished using it
func TestClose(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	fakeConn := &mockConnection{}
	fakeConn.On("Close").Return(nil).Once()

	logger := sallust.Default()

	sender := NewSender(fakeConn, 1, 1, logger)
	encoder := NewEncoderSender(sender, 1, 1, logger)
	handlers, err := NewHandlerRegistry([]HandlerConfig{})
	require.NoError(err)
	rh := NewRegistryHandler(
		func(message *wrp.Message) {},
		handlers,
		NewDownstreamSender(func(message *wrp.Message) {}, 1, 1, logger),
		1, 1, "mac:deadbeefcafe", logger)
	decoder := NewDecoderSender(rh, 1, 1, logger)

	testClient := &client{
		encoderSender: encoder,
		decoderSender: decoder,
		connection:    fakeConn,
		logger:        logger,
		done:          make(chan struct{}, 1),
	}
	err = testClient.Close()

	assert.Nil(err)
	fakeConn.AssertExpectations(t)
}

// test what happens when we get an error closing the websocket
func TestCloseBroken(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	fakeConn := &mockConnection{}

	fakeConn.On("Close").Return(ErrFoo).Once()

	logger := sallust.Default()
	sender := NewSender(fakeConn, 1, 1, logger)
	encoder := NewEncoderSender(sender, 1, 1, logger)
	handlers, err := NewHandlerRegistry([]HandlerConfig{})
	require.NoError(err)
	rh := NewRegistryHandler(
		func(message *wrp.Message) {},
		handlers,
		NewDownstreamSender(func(message *wrp.Message) {}, 1, 1, logger),
		1, 1, "mac:deadbeefcafe", logger)
	decoder := NewDecoderSender(rh, 1, 1, logger)

	testClient := &client{
		encoderSender: encoder,
		decoderSender: decoder,
		connection:    fakeConn,
		logger:        logger,
		done:          make(chan struct{}, 1),
	}
	err = testClient.Close()

	assert.NotNil(err)
	fakeConn.AssertExpectations(t)
}

// test the happy path of receiving a message from the server via websocket
// users will never make function calls to this, in the normal use case
// they simply provide a handler and let a go routine deal with this call
func TestRead(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	fakeConn := &mockConnection{}
	fakeConn.On("ReadMessage").Return(0, goodMsg, nil)

	registry, err := NewHandlerRegistry([]HandlerConfig{
		{
			Regexp: "/bar",
			Handler: &myReadHandler{
				helloMsg:      "Whaddup.",
				goodbyeMsg:    "It's dat boi Kratos.",
				handlerCalled: false,
			},
		},
	})
	require.NoError(err)
	logger := sallust.Default()
	sender := NewSender(fakeConn, 1, 1, logger)
	encoder := NewEncoderSender(sender, 1, 1, logger)

	rh := NewRegistryHandler(func(message *wrp.Message) {},
		registry,
		NewDownstreamSender(func(message *wrp.Message) {}, 1, 1, logger),
		1, 1, clientConfig.DeviceName, logger)
	decoder := NewDecoderSender(rh, 1, 1, logger)
	testClient := &client{
		deviceID:        clientConfig.DeviceName,
		userAgent:       "",
		deviceProtocols: "",
		registry:        registry,
		connection:      fakeConn,
		encoderSender:   encoder,
		decoderSender:   decoder,
		headerInfo:      nil,
		logger:          sallust.Default(),
	}

	mainWG.Add(1)
	go func() {
		testClient.read()
	}()

	mainWG.Wait()

	assert.Nil(err)
	fakeConn.AssertExpectations(t)
}
