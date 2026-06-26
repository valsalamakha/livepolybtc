package execution

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// newID returns a short, time-ordered, collision-resistant identifier suitable
// for correlating orders across the pipeline. It avoids an external UUID
// dependency.
func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to time-only uniqueness; extremely unlikely.
		return fmt.Sprintf("ord-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("ord-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b[:]))
}
