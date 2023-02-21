package kratos

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

// HandlePingMiss is a function called when we run into situations where we're
// not getting anymore pings.  The implementation of this function needs to be
// handled by the user of kratos.
type HandlePingMiss func() error

// checkPing is a function that checks that we are receiving pings within a
// given interval.  If a ping is missed, we call the client's HandlePingMiss
// function.
func (c *client) checkPing(inTimer *time.Timer, pinged <-chan string) {
	fmt.Println("in checkPing")
	defer c.wg.Done()
	// pingMiss indicates that a ping has been missed.
	pingMiss := false
	c.logger.Info("Watching socket for pings")
	count := 0
	fmt.Println("about to go into for")
	// as long as we're getting pings, we continue to loop.
	for !pingMiss {
		fmt.Println("in select")
		select {

		// if we get a done signal, we leave the function.
		case <-c.done:
			c.logger.Info("Stopped waiting for pings")
			fmt.Println("done")
			return
			// if we get a ping, make sure to reset the timer until the next ping.
		case <-pinged:
			count = 0
			fmt.Println("pinged")
			if !inTimer.Stop() {
				<-inTimer.C
			}
			c.logger.Debug("Received a ping. Resetting ping timer")

			inTimer.Reset(c.pingConfig.PingWait)

		// if we hit the timer, we've missed a ping.
		case <-inTimer.C:
			fmt.Println("inTimer")
			c.logger.Error("Ping miss, calling handler", zap.Int("count", count))
			err := c.handlePingMiss()
			if err != nil {
				c.logger.Error("Error handling ping miss:", zap.Error(err))
			}
			if count >= c.pingConfig.MaxPingMiss {
				fmt.Println("exiting ping loop")
				c.logger.Error("Ping miss, exiting ping loop")
				pingMiss = true
			}
			c.logger.Debug("Resetting ping timer")
			inTimer.Reset(c.pingConfig.PingWait)
		}
		fmt.Println("end of checkPing")
	}
}
