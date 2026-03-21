package pool

import (
	"net/http"
	"sync"
	"time"

	"github.com/htchan/goclient"
)

type RequestRecorder func(pool ClientPool, cli *http.Client, req *http.Request, resp *http.Response, err error)

func NewRequestRecorderAlwaysAddClientBack(cooldownInterval time.Duration) RequestRecorder {
	return func(pool ClientPool, cli *http.Client, req *http.Request, resp *http.Response, err error) {
		go func() {
			time.Sleep(cooldownInterval)
			pool.AddClients(cli)
		}()
	}
}

func NewRequestRecorderDropClientForContinueFailed(
	isRequestFail goclient.ResultValidator,
	failureThreshold int,
	failureCooldownInterval time.Duration,
	successCooldownInterval time.Duration,
) RequestRecorder {
	failureCounts := make(map[*http.Client]int)
	lock := new(sync.Mutex)
	return func(pool ClientPool, cli *http.Client, req *http.Request, resp *http.Response, err error) {
		lock.Lock()
		defer lock.Unlock()

		// check failure count
		cooldownInterval := successCooldownInterval

		failureCount, ok := failureCounts[cli]
		if !ok {
			failureCount = 0
		}

		// update failure count
		if isRequestFail(req, resp, err) {
			failureCount += 1
			cooldownInterval = failureCooldownInterval
		} else {
			failureCount = 0
		}

		// create go routine to add client back to pool after cooldown
		if failureCount < failureThreshold {
			failureCounts[cli] = failureCount
			go func() {
				time.Sleep(cooldownInterval)
				pool.AddClients(cli)
			}()
		} else {
			// client is being dropped — clean up its entry to prevent map leak
			delete(failureCounts, cli)
		}
	}
}
