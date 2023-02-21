package kratos

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/xmidt-org/sallust"
)

var c = ClientConfig{
	DeviceName: "mac:ffffff112233",
	HandlePingMiss: func() error {
		fmt.Println("hi")
		pingMissCount++
		return nil
	},
	ClientLogger: nil,
	PingConfig:   PingConfig{PingWait: 1000000000, MaxPingMiss: 0},
}

var pingMissCount = 0

func TestPingDone(t *testing.T) {
	pingMissCount = 0
	newClient := &client{
		deviceID:       c.DeviceName,
		handlePingMiss: c.HandlePingMiss,
		done:           make(chan struct{}, 1),
		logger:         sallust.Default(),
		pingConfig:     c.PingConfig,
		wg:             sync.WaitGroup{},
	}

	newClient.wg.Add(1)
	pinged := make(chan string)
	pingTimer := time.NewTimer(newClient.pingConfig.PingWait)

	close(newClient.done)
	newClient.checkPing(pingTimer, pinged)

	newClient.wg.Wait()

	assert.Equal(t, pingMissCount, 0)
}

func TestPing(t *testing.T) {
	pingMissCount = 0
	newClient := &client{
		deviceID:       c.DeviceName,
		handlePingMiss: c.HandlePingMiss,
		done:           make(chan struct{}, 1),
		logger:         sallust.Default(),
		pingConfig:     c.PingConfig,
		wg:             sync.WaitGroup{},
	}

	newClient.wg.Add(1)
	pinged := make(chan string)
	pingTimer := time.NewTimer(newClient.pingConfig.PingWait)

	go func() {
		pinged <- "ping"
	}()

	time.Sleep(50)

	newClient.checkPing(pingTimer, pinged)

	newClient.wg.Wait()

	assert.Equal(t, pingMissCount, 1)
}
