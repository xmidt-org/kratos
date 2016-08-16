package kratos

import (
	"bytes"
	"errors"
	"github.com/comcast/webpa-common/wrp"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

const (
	socketBufferSize = 1024
)

var (
	testClientFactory = &ClientFactory{
		DeviceName:     "mac:ffffff112233",
		FirmwareName:   "TG1682_2.1p7s1_PROD_sey",
		ModelName:      "TG1682G",
		Manufacturer:   "ARRIS Group, Inc.",
		DestinationUrl: "",
		Handlers: []HandlerRegistry{
			HandlerRegistry{
				HandlerKey: "/foo",
				Handler: &myReadHandler{
					helloMsg:   "Hello.",
					goodbyeMsg: "I am Kratos.",
				},
			},
			HandlerRegistry{
				HandlerKey: "/bar",
				Handler: &myReadHandler{
					helloMsg:   "Whaddup.",
					goodbyeMsg: "It's dat boi Kratos.",
				},
			},
			HandlerRegistry{
				HandlerKey: "/.*",
				Handler: &myReadHandler{
					helloMsg:   "Hey.",
					goodbyeMsg: "Have you met Kratos?",
				},
			},
		},
	}

	testServer *httptest.Server

	goodMsg []byte

	GoodError = errors.New("This was supposed to happen.")

	upgrader = &websocket.Upgrader{
		ReadBufferSize:  socketBufferSize,
		WriteBufferSize: socketBufferSize,
	}

	mainWG sync.WaitGroup
)

/****************** BEGIN MOCK DECLARATIONS ***********************/
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

type mockMessage struct {
	mock.Mock
}

func (m *mockMessage) WriteTo(output io.Writer) (int64, error) {
	arguments := m.Called(output)
	return arguments.Get(0).(int64), arguments.Error(1)
}

/******************* END MOCK DECLARATIONS ************************/

type myReadHandler struct {
	helloMsg   string
	goodbyeMsg string
}

func (m *myReadHandler) HandleMessage(msg interface{}) {
	// do nothing
}

func TestMain(m *testing.M) {
	testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader.Upgrade(w, r, nil)
	}))
	defer testServer.Close()

	testClientFactory.DestinationUrl = testServer.URL

	goodMsg, _ = (wrp.SimpleReqResponseMsg{
		Source:          "mac:ffffff112233/emu",
		Dest:            "/bar",
		TransactionUUID: "emu:unique",
		Payload:         []byte("the payload has reached the checkpoint"),
	}).Encode()

	os.Exit(m.Run())
}

func TestNew(t *testing.T) {
	assert := assert.New(t)
	_, err := testClientFactory.New()

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
	goodURL := testClientFactory.DestinationUrl
	testClientFactory.DestinationUrl = "broken.url"
	_, err := testClientFactory.New()

	testClientFactory.DestinationUrl = goodURL
	assert.NotNil(err)
}

func TestBadHandshake(t *testing.T) {
	assert := assert.New(t)

	brokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// do nothing so the websocket receives a bad handshake
	}))
	defer brokenServer.Close()

	testClientFactory.DestinationUrl = brokenServer.URL
	_, err := testClientFactory.New()

	testClientFactory.DestinationUrl = testServer.URL

	assert.NotNil(err)
}

// test the happy-path of sending a message through a websocket
func TestSend(t *testing.T) {
	assert := assert.New(t)
	const expectedByteCount = 42
	var buffer bytes.Buffer

	fakeConn := &mockConnection{}
	fakeMsg := &mockMessage{}

	testClient := &client{}
	testClient.connection = fakeConn

	fakeConn.On("WriteMessage", 1, buffer.Bytes()).Return(nil).Once()

	fakeMsg.On(
		"WriteTo",
		mock.MatchedBy(func(io.Writer) bool { return true }),
	).Return(int64(expectedByteCount), nil).Once()

	err := testClient.Send(fakeMsg)

	assert.Nil(err)
	fakeConn.AssertExpectations(t)
	fakeMsg.AssertExpectations(t)
}

// test what happens when a websocket fails to write a message
func TestSendBrokenWriteMessage(t *testing.T) {
	assert := assert.New(t)
	const expectedByteCount = 42
	var buffer bytes.Buffer

	fakeConn := &mockConnection{}
	fakeMsg := &mockMessage{}

	testClient := &client{}
	testClient.connection = fakeConn

	fakeConn.On("WriteMessage", 1, buffer.Bytes()).Return(GoodError).Once()

	fakeMsg.On(
		"WriteTo",
		mock.MatchedBy(func(io.Writer) bool { return true }),
	).Return(int64(expectedByteCount), nil).Once()

	err := testClient.Send(fakeMsg)

	assert.NotNil(err)
	fakeConn.AssertExpectations(t)
	fakeMsg.AssertExpectations(t)
}

// test what happens when the io.WriterTo fails to write to the buffer
func TestSendBrokenWriter(t *testing.T) {
	assert := assert.New(t)
	const expectedByteCount = 42

	fakeMsg := &mockMessage{}

	testClient := &client{}

	fakeMsg.On(
		"WriteTo",
		mock.MatchedBy(func(io.Writer) bool { return true }),
	).Return(int64(expectedByteCount), GoodError).Once()

	err := testClient.Send(fakeMsg)

	assert.NotNil(err)
	fakeMsg.AssertExpectations(t)
}

// test closing a websocket once we're finished using it
func TestClose(t *testing.T) {
	assert := assert.New(t)
	fakeConn := &mockConnection{}

	testClient := &client{}
	testClient.connection = fakeConn

	// test the happy path of closing the websocket
	fakeConn.On("Close").Return(nil).Once()

	err := testClient.Close()

	assert.Nil(err)
	fakeConn.AssertExpectations(t)
}

// test what happens when we get an error closing the websocket
func TestCloseBroken(t *testing.T) {
	assert := assert.New(t)
	fakeConn := &mockConnection{}

	testClient := &client{}
	testClient.connection = fakeConn

	fakeConn.On("Close").Return(GoodError).Once()

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

	testClient := &client{
		deviceId:        testClientFactory.DeviceName,
		userAgent:       "",
		deviceProtocols: "",
		handlers:        testClientFactory.Handlers,
		connection:      fakeConn,
		headerInfo:      nil,
	}

	fakeConn.On("ReadMessage").Return(0, goodMsg, nil).Once()

	err := testClient.read()

	assert.Nil(err)
	fakeConn.AssertExpectations(t)
}

// test what happens when the call to `websocket.Conn.ReadMessage` fails
func TestReadBrokenReadMessage(t *testing.T) {
	assert := assert.New(t)
	fakeConn := &mockConnection{}

	testClient := &client{
		deviceId:        testClientFactory.DeviceName,
		userAgent:       "",
		deviceProtocols: "",
		handlers:        testClientFactory.Handlers,
		connection:      fakeConn,
		headerInfo:      nil,
	}

	fakeConn.On("ReadMessage").Return(0, goodMsg, GoodError).Once()

	err := testClient.read()

	assert.NotNil(err)
	fakeConn.AssertExpectations(t)
}

// test what happens when we receive a message type that wrp does not expect
func TestReadBrokenMessageType(t *testing.T) {
	assert := assert.New(t)
	fakeConn := &mockConnection{}

	// this shouldn't work because wrp specifies the types of variables it wants
	// and this isn't one of those variables
	brokenMsg := []byte("This shouldn't work.")

	testClient := &client{}
	testClient.connection = fakeConn

	fakeConn.On("ReadMessage").Return(0, brokenMsg, nil).Once()

	err := testClient.read()

	assert.NotNil(err)
	fakeConn.AssertExpectations(t)
}
