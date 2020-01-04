package kratos

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/xmidt-org/wrp-go/wrp"
)

const (
	StatusDeviceDisconnected int = 523
	StatusDeviceTimeout      int = 524
)

type Message struct {
	Code int    `json:"code"`
	Body string `json:"body"`
}

// statusCode follows the go-kit convention.  Errors and other objects that implement
// this interface are allowed to supply an HTTP response status code.
type StatusCoder interface {
	StatusCode() int
}

type MessageBodyer interface {
	MessageBody() string
}

func (msg Message) String() string {
	return fmt.Sprintf("%d:%s", msg.Code, msg.Body)
}

// httpError can provide a StatusCode or MessageBody from the response received.
type httpError struct {
	Message  Message
	SubError error
}

func createHTTPError(resp *http.Response, err error) *httpError {
	var msg Message
	defer resp.Body.Close()
	data, _ := ioutil.ReadAll(resp.Body)
	json.Unmarshal(data, &msg)

	if msg.Body == "" {
		switch resp.StatusCode {
		case StatusDeviceDisconnected:
			msg.Body = "ErrorDeviceBusy"
		case StatusDeviceTimeout:
			msg.Body = "ErrorTransactionsClosed/ErrorTransactionsAlreadyClosed/ErrorDeviceClosed"
		default:
			msg.Body = http.StatusText(msg.Code)
		}
	}

	return &httpError{
		Message:  msg,
		SubError: err,
	}
}

func (e *httpError) Error() string {
	return fmt.Sprintf("message: %s with error: %s", e.Message, e.SubError.Error())
}

func (e *httpError) StatusCode() int {
	return e.Message.Code
}

func (e *httpError) MessageBody() string {
	return e.Message.Body
}

// errors provides a way to supply and parse a list of errors
type errorList []error

func (es errorList) Error() string {
	errStrings := []string{}
	for _, err := range es {
		errStrings = append(errStrings, err.Error())
	}
	return fmt.Sprintf("multiple errors: [%v]", strings.Join(errStrings, ","))
}

func (es errorList) Errors() []error {
	return es
}

func CreateErrorWRP(transaction string, dest string, src string, statusCode int64, err error) *wrp.Message {
	response := wrp.Message{
		Type:            wrp.SimpleRequestResponseMessageType,
		Destination:     dest,
		Source:          src,
		ContentType:     "application/json",
		Payload:         []byte(fmt.Sprintf(`{"err":"%s"}`, err.Error())),
		TransactionUUID: transaction,
	}
	response.SetStatus(statusCode)
	return &response
}
