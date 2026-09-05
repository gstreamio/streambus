package bench

import (
	"context"
	"time"

	"github.com/gstreamio/streambus/pkg/client"
	"github.com/gstreamio/streambus/pkg/protocol"
)

// pollBackoff is how long a consumer loop in this package waits before
// retrying after an empty GroupConsumer.Poll. Poll does one fetch pass and
// returns immediately whether or not anything was available - unlike the
// older single-partition Consumer.Poll, it takes no timeout and does not
// block waiting for new data - so a loop that wants to wait for messages
// has to back off itself instead of spinning the CPU.
const pollBackoff = 5 * time.Millisecond

// healthCheckTimeout bounds the reachability probe in newBenchClient.
const healthCheckTimeout = 3 * time.Second

// newBenchClient builds a client pointed at brokers with the given request
// timeout, then confirms the first broker actually answers before handing
// the client back.
//
// client.New only validates the config - it never dials, so a struct that
// passes Validate() succeeds regardless of whether anything is listening.
// Every call site here relies on an error from newBenchClient to mean "no
// broker, skip this benchmark" (via b.Skipf), so without an explicit
// reachability check every benchmark would instead fail deep inside a
// goroutine the first time it actually tried to send or poll - turning a
// clean, expected skip in a broker-less CI run into a hard test failure.
//
// Building on client.DefaultConfig() (rather than a bare struct literal)
// keeps ConnectTimeout, MaxConnectionsPerBroker and the rest of
// Config.Validate()'s requirements satisfied, too - a literal that only sets
// Brokers and a timeout field fails validation before ever reaching this
// check.
func newBenchClient(brokers []string, requestTimeout time.Duration) (*client.Client, error) {
	cfg := client.DefaultConfig()
	cfg.Brokers = brokers
	cfg.RequestTimeout = requestTimeout

	c, err := client.New(cfg)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	defer cancel()
	if err := c.HealthCheck(ctx, brokers[0]); err != nil {
		_ = c.Close()
		return nil, err
	}

	return c, nil
}

// countMessages sums the messages returned by one GroupConsumer.Poll call
// across every topic and partition in the result, so a benchmark that counts
// messages consumed still counts them all instead of e.g. just len(result).
func countMessages(result map[string]map[int32][]protocol.Message) int {
	n := 0
	for _, byPartition := range result {
		for _, msgs := range byPartition {
			n += len(msgs)
		}
	}
	return n
}
