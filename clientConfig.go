package kratos

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/websocket"
	"github.com/xmidt-org/webpa-common/device"
	"github.com/xmidt-org/webpa-common/logging"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = time.Duration(10) * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
)

var (
	errNilHandlePingMiss = errors.New("HandlePingMiss should not be nil")
)

// ClientConfig is the configuration to provide when making a new client.
type ClientConfig struct {
	DeviceName            string
	FirmwareName          string
	ModelName             string
	Manufacturer          string
	DestinationURL        string
	CertificatesPATH      string
	CallPetasosForTalaria bool
	OutboundQueue         QueueConfig
	WRPEncoderQueue       QueueConfig
	WRPDecoderQueue       QueueConfig
	HandlerRegistryQueue  QueueConfig
	HandleMsgQueue        QueueConfig
	Handlers              []HandlerConfig
	HandlePingMiss        HandlePingMiss
	ClientLogger          log.Logger
	PingConfig            PingConfig
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

	newConnection, connectionURL, err := createConnection(inHeader, config)

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
	connectionURL = strings.TrimPrefix(connectionURL[len("ws://"):], ":")

	var logger log.Logger
	if config.ClientLogger != nil {
		logger = config.ClientLogger
	} else {
		logger = logging.DefaultLogger()
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
		logging.Warn(newClient.logger).Log(logging.MessageKey(), "failed to initialize all handlers for registry", logging.ErrorKey(), err.Error())
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
func createConnection(headerInfo *clientHeader, config ClientConfig) (connection *websocket.Conn, wsURL string, err error) {
	_, err = device.ParseID(headerInfo.deviceName)

	if err != nil {
		return nil, "", err
	}

	tlsConfig, err := GetTLSConfig(strings.Split(headerInfo.deviceName, ":")[1], config.CertificatesPATH)
	var talariaInstance = config.DestinationURL
	if config.CallPetasosForTalaria {
		talariaInstance, err = getTalariaInstance(config, tlsConfig)
	}

	dialer := &websocket.Dialer{}
	if err == nil {
		// Set the TLS configuration of the dialer
		dialer.TLSClientConfig = tlsConfig
	}

	// make a header and put some data in that (including MAC address)
	// TODO: find special function for user agent
	headers := make(http.Header)
	headers.Add("X-Webpa-Device-Name", headerInfo.deviceName)
	headers.Add("X-Webpa-Firmware-Name", headerInfo.firmwareName)
	headers.Add("X-Webpa-Model-Name", headerInfo.modelName)
	headers.Add("X-Webpa-Manufacturer", headerInfo.manufacturer)

	// make sure destUrl's protocol is websocket (ws)
	wsURL = strings.Replace(talariaInstance, "http", "ws", 1)

	// creates a new client connection given the URL string
	connection, resp, err := dialer.Dial(wsURL, headers)

	for err == websocket.ErrBadHandshake && resp != nil && resp.StatusCode == http.StatusTemporaryRedirect {
		fmt.Println(err)
		// Get url to which we are redirected and reconfigure it
		wsURL = strings.Replace(resp.Header.Get("Location"), "http", "ws", 1)

		connection, resp, err = dialer.Dial(wsURL, headers)
	}

	if err != nil {
		if resp != nil {
			err = createHTTPError(resp, err)
		}
		return nil, "", err
	}

	return connection, wsURL, nil
}

func getTalariaInstance(config ClientConfig, tlsConfig *tls.Config) (string, error) {

	// Create HTTP client with custom transport

	tr := &http.Transport{
		TLSClientConfig:    tlsConfig,
		DisableCompression: true,
		DisableKeepAlives:  true,
	}
	client := &http.Client{Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// Create HTTP request with headers
	req, err := http.NewRequest("GET", config.DestinationURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Webpa-Device-Name", config.DeviceName)

	// Send HTTP request and get response
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	pattern := `((?:https?://|www\d{0,3}[.]|[a-z0-9.\-]+[.][a-z]{2,4}/)(?:[^\s()<>]+|\(([^\s()<>]+|(\([^\s()<>]+\)))*\))+(?:\(([^\s()<>]+|(\([^\s()<>]+\)))*\)|[^\s!()\[\]{};:\'".,<>?«»“”‘’]))`

	re := regexp.MustCompile(pattern)
	match := re.FindString(string(body))
	return match, nil
}

func GetTLSConfig(macAddress string, certificatesPath string) (*tls.Config, error) {
	certFile := fmt.Sprintf("%s/%s-client.crt", certificatesPath, macAddress)
	keyFile := fmt.Sprintf("%s/%s-key.pem", certificatesPath, macAddress)
	caFile := fmt.Sprintf("%s/ca.crt", certificatesPath)

	// Try reading the certificate files with the prefix of the provided macaddress
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		// If that fails, try reading the certificate files without the prefix
		certFile = fmt.Sprintf("%s/client.crt", certificatesPath)
		keyFile = fmt.Sprintf("%s/key.pem", certificatesPath)
		cert, err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}
	}

	caCert, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	return &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{cert},
	}, nil

}
