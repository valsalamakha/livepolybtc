package app

import (
	"fmt"
	"time"
)

// heartbeatMsg formats a compact health summary for the event log.
func heartbeatMsg(connected bool, since time.Duration, btcStale, killed bool) string {
	return fmt.Sprintf("ws_connected=%v last_update=%s btc_stale=%v kill_switch=%v",
		connected, since.Round(time.Millisecond), btcStale, killed)
}
