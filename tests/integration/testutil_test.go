package integration

import "os"

// testBrokers returns the broker address(es) to use for integration tests.
// Defaults to localhost:9092 but can be overridden with STREAMBUS_TEST_BROKER
// so these tests don't have to run against whatever broker already owns 9092.
func testBrokers() []string {
	if v := os.Getenv("STREAMBUS_TEST_BROKER"); v != "" {
		return []string{v}
	}
	return []string{"localhost:9092"}
}
