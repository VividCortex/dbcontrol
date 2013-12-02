// Copyright (c) 2013 VividCortex. Please see the LICENSE file for license terms.

package dbcontrol

import (
	"sync"
)

var (
	concurrency    int
	concurrencyMux sync.RWMutex
)

// SetConcurrency sets the maximum number of simultaneous connections per
// database, for all subsequent DBs that are opened. (Note that this doesn't
// affect databases that are already open.) No capping on the number of
// connections is performed if concurrency is set to a non-possitive value.
func SetConcurrency(count int) {
	concurrencyMux.Lock()
	defer concurrencyMux.Unlock()

	if count > 0 {
		concurrency = count
	} else {
		concurrency = 0
	}
}

// Concurrency returns the concurrency setting. See SetConcurrency().
func Concurrency() int {
	concurrencyMux.RLock()
	defer concurrencyMux.RUnlock()
	return concurrency
}
