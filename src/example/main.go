package main

import (
	"fmt"
	"github.com/comcast/webpa-common/wrp"
	"github.com/nu7hatch/gouuid"
	"kratos"
	"sync"
)

var (
	mainWG sync.WaitGroup
)

type myReadHandler struct {
	helloMsg   string
	goodbyeMsg string
}

func (m *myReadHandler) HandleMessage(msg interface{}) {
	fmt.Println()
	fmt.Println(m.helloMsg)
	fmt.Println(m.goodbyeMsg)
	fmt.Println(msg)

	mainWG.Done()
}

func main() {
	// right now the key in kratos for the handler is the MAC address,
	// so make sure that's what you pass in otherwise you won't ever read anything
	client, err := (&kratos.ClientFactory{
		DeviceName:     "mac:ffffff112233",
		FirmwareName:   "TG1682_2.1p7s1_PROD_sey",
		ModelName:      "TG1682G",
		Manufacturer:   "ARRIS Group, Inc.",
		DestinationUrl: "https://fabric-cd.webpa.comcast.net:8080/api/v2/device",
		Handlers: []kratos.HandlerRegistry{
			kratos.HandlerRegistry{
				HandlerKey: "/foo",
				Handler: &myReadHandler{
					helloMsg:   "Hello.",
					goodbyeMsg: "I am Kratos.",
				},
			},
			kratos.HandlerRegistry{
				HandlerKey: "/bar",
				Handler: &myReadHandler{
					helloMsg:   "Hi.",
					goodbyeMsg: "My name is Kratos.",
				},
			},
			kratos.HandlerRegistry{
				HandlerKey: ".*",
				Handler: &myReadHandler{
					helloMsg:   "Hey.",
					goodbyeMsg: "Have you met Kratos?",
				},
			},
		},
	}).New()
	if err != nil {
		fmt.Println("Error making client: ", err)
	}

	// generate a uuid for use below in the clientMessage
	u4, err := uuid.NewV4()
	if err != nil {
		fmt.Println("Error generating uuid: ", err)
	}

	// construct a client message for us to send to the server
	myMessage := wrp.SimpleReqResponseMsg{
		Source:          "mac:ffffff112233/emu",
		Dest:            "event:device-status/bla/bla",
		TransactionUUID: "emu:" + u4.String(),
		Payload:         []byte("the payload has reached the checkpoint"),
	}

	if err = client.Send(wrp.WriterTo(myMessage)); err != nil {
		fmt.Println("Error sending message: ", err)
	}

	mainWG.Add(1)
	mainWG.Wait()

	if err = client.Close(); err != nil {
		fmt.Println("Error closing connection: ", err)
	}
}
