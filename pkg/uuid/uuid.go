package uuid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func NewV4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32]), nil
}

func MustV4() string {
	id, err := NewV4()
	if err != nil {
		panic(err)
	}
	return id
}
