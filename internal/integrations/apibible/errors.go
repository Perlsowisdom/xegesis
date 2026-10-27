package apibible

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrBadKey     = errors.New("API.Bible rejected the API key")
	ErrForbidden  = errors.New("that Bible is not included in your API.Bible plan")
	ErrNotFound   = errors.New("not found on API.Bible")
	ErrRateLimit  = errors.New("API.Bible rate limit reached")
	ErrBadRequest = errors.New("API.Bible rejected the request")
	ErrNoKey      = errors.New("no API.Bible key is set; run apibible.set_key")
)

// APIError is a failed API.Bible response. It unwraps to one of the Err values above, so callers
// use errors.Is. It never contains the API key.
type APIError struct {
	Status     int
	Message    string
	RetryAfter time.Duration // set for rate limits when the server says how long to wait
	kind       error
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("%v (HTTP %d)", e.kind, e.Status)
	}
	return fmt.Sprintf("%v (HTTP %d: %s)", e.kind, e.Status, e.Message)
}

func (e *APIError) Unwrap() error { return e.kind }

func kindFor(status int) error {
	switch status {
	case 401:
		return ErrBadKey
	case 403:
		return ErrForbidden
	case 404:
		return ErrNotFound
	case 429:
		return ErrRateLimit
	case 400:
		return ErrBadRequest
	}
	return nil
}
