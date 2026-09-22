// Package randomstream defines explicit independent pseudo-random streams for
// the NEW reconstruction. It does not reproduce the lost experiment generator.
package randomstream

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
)

const Version = "sha256-namespaced-go-rand-v1"

// New hashes an unambiguous tuple, not concatenated unseparated strings.
// The namespace must identify environment, observation or forecast use.
func New(master uint64, namespace string, keys ...string) (*rand.Rand, error) {
	if namespace == "" {
		return nil, fmt.Errorf("explicit random-stream namespace required")
	}
	tuple := struct {
		Version   string
		Master    uint64
		Namespace string
		Keys      []string
	}{Version, master, namespace, keys}
	b, err := json.Marshal(tuple)
	if err != nil {
		return nil, err
	}
	s := sha256.Sum256(b)
	return rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(s[:8])))), nil
}
