//go:build !unix

package nightshift

import (
	"os"
	"time"
)

// Non-Unix terminals keep the safe blocking input path. The runtime disables
// live cursor updates when this helper reports that timed polling is absent.
func waitForInput(*os.File, time.Duration) (bool, error) {
	return false, errInputPollingUnavailable
}
