package kratos

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/webpa-common/device"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/wrp-go/wrp"
)

const (
	socketBufferSize = 1024
)

var (
	// handlerCalled is true and used for synchronization, true meaning that
	// we don't need to worry about synchronization (mainly for calls to TestRead)
	testClientFactory = &ClientFactory{
		DeviceName:     "mac:ffffff112233",
		FirmwareName:   "TG1682_2.1p7s1_PROD_sey",
		ModelName:      "TG1682G",
		Manufacturer:   "ARRIS Group, Inc.",
		DestinationURL: "",
		Handlers: []HandlerRegistry{
			{
				HandlerKey: "/foo",
				Handler: &myReadHandler{
					helloMsg:      "Hello.",
					goodbyeMsg:    "I am Kratos.",
					handlerCalled: true,
				},
			},
			{
				HandlerKey: "/bar",
				Handler: &myReadHandler{
					helloMsg:      "Whaddup.",
					goodbyeMsg:    "It's dat boi Kratos.",
					handlerCalled: true,
				},
			},
			{
				HandlerKey: "/.*",
				Handler: &myReadHandler{
					helloMsg:      "Hey.",
					goodbyeMsg:    "Have you met Kratos?",
					handlerCalled: true,
				},
			},
		},
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

func (m *myReadHandler) HandleMessage(msg interface{}) {
	if !m.handlerCalled {
		mainWG.Done()
		m.handlerCalled = true
	}
}

func TestMain(m *testing.M) {
	testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader.Upgrade(w, r, nil)
	}))
	defer testServer.Close()

	testClientFactory.DestinationURL = testServer.URL

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
	msg := fmt.Sprintf("Could not process device request: %s", device.ErrorDeviceClosed)

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

	testClientFactory.DestinationURL = brokenServer.URL
	_, err := testClientFactory.New()

	testClientFactory.DestinationURL = testServer.URL

	assert.NotNil(err)
	expected := fmt.Sprintf("message: %s with error: %s", Message{code, msg}, websocket.ErrBadHandshake)
	assert.Equal(expected, err.Error())
}

func TestNew(t *testing.T) {
	assert := assert.New(t)
	testClient, err := testClientFactory.New()

	assert.Equal("127.0.0.1", testClient.Hostname())
	assert.Nil(err)
}

func TestNewBrokenMAC(t *testing.T) {
	assert := assert.New(t)
	goodMac := testClientFactory.DeviceName
	testClientFactory.DeviceName = "broken:mac"
	_, err := testClientFactory.New()

	testClientFactory.DeviceName = goodMac
	assert.NotNil(err)
}

func TestNewBrokenURL(t *testing.T) {
	assert := assert.New(t)
	goodURL := testClientFactory.DestinationURL
	testClientFactory.DestinationURL = "broken.url"
	_, err := testClientFactory.New()

	testClientFactory.DestinationURL = goodURL
	assert.NotNil(err)
}

func TestBadHandshake(t *testing.T) {
	assert := assert.New(t)

	brokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// do nothing so the websocket receives a bad handshake
	}))
	defer brokenServer.Close()

	testClientFactory.DestinationURL = brokenServer.URL
	_, err := testClientFactory.New()

	testClientFactory.DestinationURL = testServer.URL

	assert.NotNil(err)
}

func TestCheckPingTimeout(t *testing.T) {
	assert := assert.New(t)
	timesCalled := 0

	fakeClient := &mockClient{}
	fakeClient.On("Close").Return(nil).Once()

	testPingMissHandler := pingMissHandler{
		handlePingMiss: func() error {
			timesCalled++
			return nil
		},
		Logger: logging.New(nil),
	}

	pingTimer := time.NewTimer(time.Duration(1) * time.Second)
	pinged := make(chan string)

	testPingMissHandler.checkPing(pingTimer, pinged, fakeClient)

	assert.Equal(1, timesCalled)
	fakeClient.AssertExpectations(t)
}

// test the happy-path of sending a message through a websocket
func TestSend(t *testing.T) {
	assert := assert.New(t)
	fakeConn := &mockConnection{}
	fakeConn.On("WriteMessage", websocket.BinaryMessage, mock.AnythingOfType("[]uint8")).Return(nil).Once()

	myMessage := wrp.SimpleRequestResponse{
		Source:      "mac:ffffff112233/emu",
		Destination: "event:device-status/bla/bla",
		Payload:     []byte("the payload has reached the checkpoint"),
	}

	testClient := &client{
		connection: fakeConn,
		Logger:     logging.New(nil),
	}

	err := testClient.Send(myMessage)

	assert.Nil(err)
	fakeConn.AssertExpectations(t)
}

// test what happens when a websocket fails to write a message
func TestSendBrokenWriteMessage(t *testing.T) {
	assert := assert.New(t)

	fakeConn := &mockConnection{}
	fakeConn.On("WriteMessage", websocket.BinaryMessage, mock.AnythingOfType("[]uint8")).Return(ErrFoo).Once()

	testClient := &client{
		connection: fakeConn,
		Logger:     logging.New(nil),
	}

	err := testClient.Send(nil)

	assert.NotNil(err)
	fakeConn.AssertExpectations(t)
}

// test the happy path of closing a websocket once we're finished using it
func TestClose(t *testing.T) {
	assert := assert.New(t)

	fakeConn := &mockConnection{}
	fakeConn.On("Close").Return(nil).Once()

	testClient := &client{
		connection: fakeConn,
		Logger:     logging.New(nil),
	}

	err := testClient.Close()

	assert.Nil(err)
	fakeConn.AssertExpectations(t)
}

// test what happens when we get an error closing the websocket
func TestCloseBroken(t *testing.T) {
	assert := assert.New(t)
	fakeConn := &mockConnection{}

	fakeConn.On("Close").Return(ErrFoo).Once()

	testClient := &client{
		connection: fakeConn,
		Logger:     logging.New(nil),
	}

	err := testClient.Close()

	assert.NotNil(err)
	fakeConn.AssertExpectations(t)
}

// test the happy path of receiving a message from the server via websocket
// users will never make function calls to this, in the normal use case
// they simply provide a handler and let a go routine deal with this call
func TestRead(t *testing.T) {
	assert := assert.New(t)

	fakeConn := &mockConnection{}
	fakeConn.On("ReadMessage").Return(0, goodMsg, nil)

	testClient := &client{
		deviceID:        testClientFactory.DeviceName,
		userAgent:       "",
		deviceProtocols: "",
		handlers: []HandlerRegistry{
			{
				HandlerKey: "/bar",
				Handler: &myReadHandler{
					helloMsg:      "Whaddup.",
					goodbyeMsg:    "It's dat boi Kratos.",
					handlerCalled: false,
				},
			},
		},
		connection: fakeConn,
		headerInfo: nil,
		Logger:     logging.New(nil),
	}

	testClient.handlers[0].keyRegex, _ = regexp.Compile(testClient.handlers[0].HandlerKey)

	mainWG.Add(1)
	var err error
	go func() {
		err = testClient.read()
	}()

	mainWG.Wait()

	assert.Nil(err)
	fakeConn.AssertExpectations(t)
}
