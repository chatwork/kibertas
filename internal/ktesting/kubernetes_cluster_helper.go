package ktesting

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/mumoshu/testkit"
)

const clusterIDLen = 5

// WithRandomClusterID returns an option function that sets
// a fixed-length random lowercase hex cluster ID (length=5).
func WithRandomClusterID() func(c *testkit.KubernetesClusterConfig) {
	return func(c *testkit.KubernetesClusterConfig) {
		// Generate 3 random bytes -> 6 hex chars, then trim to 5.
		id, err := randomHex(3)
		if err != nil {
			c.ID = "aaaaa" // safe fallback on error
			return
		}
		if len(id) > clusterIDLen {
			id = id[:clusterIDLen]
		}
		c.ID = id
	}
}

// randomHex returns a hex string generated from n random bytes.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
