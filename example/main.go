package main

import (
	"fmt"
	"sync"

	"github.com/xmidt-org/kratos"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/wrp-go/v3"
)

var (
	mainWG sync.WaitGroup
)

type myReadHandler struct {
	helloMsg   string
	goodbyeMsg string
}

func (m *myReadHandler) HandleMessage(msg *wrp.Message) *wrp.Message {
	fmt.Println()
	fmt.Println(m.helloMsg)
	fmt.Println(m.goodbyeMsg)
	fmt.Println(msg)
	mainWG.Done()
	return msg
}

func (m *myReadHandler) Close() {
	fmt.Println("close")
}

func main() {
	// right now the key in kratos for the handler is the MAC address,
	// so make sure that's what you pass in otherwise you won't ever read anything
	client, err := kratos.NewClient(kratos.ClientConfig{
		DeviceName:     "mac:deadbeefcafe",
		FirmwareName:   "TG1682_2.1p7s1_PROD_sey",
		ModelName:      "TG1682G",
		Manufacturer:   "ARRIS Group, Inc.",
		DestinationURL: "http://localhost:6200/api/v2/device",
		Handlers: []kratos.HandlerConfig{
			{
				Regexp: "/foo",
				Handler: &myReadHandler{
					helloMsg:   "Hello.",
					goodbyeMsg: "I am Kratos.",
				},
			},
			{
				Regexp: "/bar",
				Handler: &myReadHandler{
					helloMsg:   "Hi.",
					goodbyeMsg: "My name is Kratos.",
				},
			},
			{
				Regexp: ".*",
				Handler: &myReadHandler{
					helloMsg:   "Hey.",
					goodbyeMsg: "Have you met Kratos?",
				},
			},
		},
		HandlePingMiss: func() error {
			fmt.Println("We missed the ping!")
			return nil
		},
		ClientLogger: logging.New(nil),
	})
	if err != nil {
		fmt.Println("Error making client: ", err)
	}

	optionalUUID := "IamOptional"
	if err != nil {
		fmt.Println("Error generating uuid: ", err)
	}

	// construct a client message for us to send to the server
	myMessage := &wrp.Message{
		Source:          "mac:ffffff112233/emu",
		Destination:     "event:device-status/bla/bla",
		TransactionUUID: "emu:" + optionalUUID,
		Payload:         []byte("the payload has reached the checkpoint"),
	}

	client.Send(myMessage)

	mainWG.Add(1)
	mainWG.Wait()

	if err = client.Close(); err != nil {
		fmt.Println("Error closing connection: ", err)
	}
}
