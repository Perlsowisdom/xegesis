package store

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewID returns a ULID: 48 bits of milliseconds then 80 random bits, in Crockford base 32.
// IDs made later sort after IDs made earlier.
func NewID() string {
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	for i := 0; i < 6; i++ {
		b[i] = byte(ms >> (40 - 8*i))
	}
	if _, err := rand.Read(b[6:]); err != nil {
		panic(err)
	}
	var out [26]byte
	// 128 bits as 26 five-bit groups; the first group holds the top 3 bits.
	var acc uint64
	bits := 0
	pos := 25
	for i := 15; i >= 0; i-- {
		acc |= uint64(b[i]) << bits
		bits += 8
		for bits >= 5 && pos >= 0 {
			out[pos] = crockford[acc&31]
			acc >>= 5
			bits -= 5
			pos--
		}
	}
	out[0] = crockford[acc&31]
	return string(out[:])
}

// Clock is a hybrid logical clock. Its timestamps order changes across devices even when their
// wall clocks disagree. Format: 13-digit milliseconds, 5-digit counter, device ID.
type Clock struct {
	mu      sync.Mutex
	device  string
	ms      int64
	counter int
	now     func() int64
}

func NewClock(device string) *Clock {
	return &Clock{device: device, now: func() int64 { return time.Now().UnixMilli() }}
}

// Now returns a timestamp later than every one this clock has made or observed.
func (c *Clock) Now() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.now(); t > c.ms {
		c.ms, c.counter = t, 0
	} else {
		c.counter++
	}
	return fmt.Sprintf("%013d.%05d.%s", c.ms, c.counter, c.device)
}

// Observe moves the clock past a timestamp received from another device.
func (c *Clock) Observe(hlc string) error {
	parts := strings.SplitN(hlc, ".", 3)
	if len(parts) != 3 {
		return fmt.Errorf("bad clock value %q", hlc)
	}
	ms, err1 := strconv.ParseInt(parts[0], 10, 64)
	n, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return fmt.Errorf("bad clock value %q", hlc)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ms > c.ms || (ms == c.ms && n > c.counter) {
		c.ms, c.counter = ms, n
	}
	return nil
}
