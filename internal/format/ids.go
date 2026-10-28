package format

import (
	"crypto/rand"
	"io"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewBlockID returns prefix + "_" + 12 random Crockford base32 characters.
func NewBlockID(random io.Reader, prefix string) string {
	if random == nil {
		random = rand.Reader
	}
	buf := make([]byte, 12)
	if _, err := io.ReadFull(random, buf); err != nil {
		panic(err)
	}
	for i, b := range buf {
		buf[i] = crockford[b&31]
	}
	return prefix + "_" + string(buf)
}
