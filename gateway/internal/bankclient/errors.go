package bankclient

import (
	"encoding/json"
	"fmt"
)

// Error is a structured failure returned BY the bank (a real HTTP response
// came back, just not a 2xx) — distinct from transport errors (timeout,
// connection refused), which doRequest wraps separately.
type Error struct {
	StatusCode int
	Code       ErrorCode
	Message    string
	Retryable  bool
}

func (e *Error) Error() string {
	return fmt.Sprintf("bank error: %s (code=%s, status=%d)", e.Message, e.Code, e.StatusCode)
}

// Per the spec: 500 is the deliberately-injected transient failure (5% of
// requests) — retryable. 400 (validation/business errors) and 402
// (insufficient funds) are declines; retrying changes nothing.
func isRetryableStatus(status int) bool {
	return status >= 500
}

func parseErrorResponse(status int, body []byte) *Error {
	var wire struct {
		Error   ErrorCode `json:"error"`
		Message string    `json:"message"`
	}
	_ = json.Unmarshal(body, &wire) // best-effort — malformed error body shouldn't itself panic error handling

	return &Error{
		StatusCode: status,
		Code:       wire.Error,
		Message:    wire.Message,
		Retryable:  isRetryableStatus(status),
	}
}
