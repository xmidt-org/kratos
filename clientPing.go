package kratos

import (
	"time"

	"github.com/xmidt-org/webpa-common/logging"
)

// HandlePingMiss is a function called when we run into situations where we're
// not getting anymore pings.  The implementation of this function needs to be
// handled by the user of kratos.
type HandlePingMiss func() error

// checkPing is a function that checks that we are receiving pings within a
// given interval.  If a ping is missed, we call the client's HandlePingMiss
// function.
func (c *client) checkPing(inTimer *time.Timer, pinged <-chan string) {
	defer c.wg.Done()
	// pingMiss indicates that a ping has been missed.
	pingMiss := false
	logging.Info(c.logger).Log(logging.MessageKey(), "Watching socket for pings")
	count := 0
	// as long as we're getting pings, we continue to loop.
	for !pingMiss {
		select {
		// if we get a done signal, we leave the function.
		case <-c.done:
			logging.Info(c.logger).Log(logging.MessageKey(), "Stopped waiting for pings")
			return
		// if we hit the timer, we've missed a ping.
		case <-inTimer.C:
			logging.Error(c.logger).Log(logging.MessageKey(), "Ping miss, calling handler", "count", count)
			err := c.handlePingMiss()
			if err != nil {
				logging.Info(c.logger).Log(logging.MessageKey(), "Error handling ping miss:", logging.ErrorKey(), err)
			}
			if count >= c.pingConfig.MaxPingMiss {
				// TOOD:// reestablish connection?
				logging.Error(c.logger).Log(logging.MessageKey(), "Ping miss, exiting ping loop")
				pingMiss = true
			}
			logging.Debug(c.logger).Log(logging.MessageKey(), "Resetting ping timer")
			inTimer.Reset(c.pingConfig.PingWait)
		// if we get a ping, make sure to reset the timer until the next ping.
		case <-pinged:
			count = 0
			if !inTimer.Stop() {
				<-inTimer.C
			}
			logging.Debug(c.logger).Log(logging.MessageKey(), "Received a ping. Resetting ping timer")
			inTimer.Reset(c.pingConfig.PingWait)
		}
	}
}
