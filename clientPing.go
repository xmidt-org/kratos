package kratos

import (
	"time"

	"github.com/xmidt-org/webpa-common/logging"
)

const (
	// Time allowed to wait in between pings
	pingWait = time.Duration(60) * time.Second
)

// HandlePingMiss is a function called when we run into situations where we're not getting anymore pings
// the implementation of this function needs to be handled by the user of kratos
type HandlePingMiss func() error

func (c *client) checkPing(inTimer *time.Timer, pinged <-chan string) {
	defer c.wg.Done()
	pingMiss := false
	logging.Info(c.logger).Log(logging.MessageKey(), "Watching socket for pings")

	for !pingMiss {
		select {
		case <-c.done:
			logging.Info(c.logger).Log(logging.MessageKey(), "Stopped waiting for pings")
			return
		case <-inTimer.C:
			logging.Error(c.logger).Log(logging.MessageKey(), "Ping miss, calling handler")
			pingMiss = true
			err := c.handlePingMiss()
			if err != nil {
				logging.Info(c.logger).Log(logging.MessageKey(), "Error handling ping miss:", logging.ErrorKey(), err)
			}
			logging.Debug(c.logger).Log(logging.MessageKey(), "Resetting ping timer")
			inTimer.Reset(pingWait)
		case <-pinged:
			if !inTimer.Stop() {
				<-inTimer.C
			}
			logging.Debug(c.logger).Log(logging.MessageKey(), "Received a ping. Resetting ping timer")
			inTimer.Reset(pingWait)
		}
	}
}
