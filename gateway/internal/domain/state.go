package domain

import (
	"fmt"
)

// PaymentState represents the current state of a payment in its lifecycle
type PaymentState string

const (
	StatePending    PaymentState = "pending"
	StateAuthorized PaymentState = "authorized"
	StateCaptured   PaymentState = "captured"
	StateVoided     PaymentState = "voided"
	StateRefunded   PaymentState = "refunded"
	StateFailed     PaymentState = "failed"
)

// String returns the string representation of the payment state
func (s PaymentState) String() string {
	return string(s)
}

func (s PaymentState) IsValid() bool {
	switch s {
	case StatePending, StateAuthorized, StateCaptured, StateVoided, StateRefunded, StateFailed:
		return true
	default:
		return false
	}
}

// ValidTransitions defines the allowed state transitions for payments
// This enforces the payment lifecycle state machine
var ValidTransitions = map[PaymentState][]PaymentState{
	StatePending: {
		StateAuthorized, // Authorization succeeded
		StateFailed,     // Authorization failed
	},
	StateAuthorized: {
		StateCaptured, // Funds captured
		StateVoided,   // Authorization voided
		StateFailed,   // Capture or void failed
	},
	StateCaptured: {
		StateRefunded, // Payment refunded — money already moved, so captured can never fail
	},
	StateVoided:   {}, // Terminal state
	StateRefunded: {}, // Terminal state
	StateFailed:   {}, // Terminal state
}

// ValidateTransition checks if a state transition is valid according to the state machine
func ValidateTransition(from, to PaymentState) error {
	if !from.IsValid() {
		return fmt.Errorf("invalid source state: %s", from)
	}
	if !to.IsValid() {
		return fmt.Errorf("invalid target state: %s", to)
	}

	allowedTransitions, exists := ValidTransitions[from]
	if !exists {
		return fmt.Errorf("no transitions allowed from state: %s", from)
	}

	for _, allowed := range allowedTransitions {
		if allowed == to {
			return nil
		}
	}

	return fmt.Errorf("invalid transition from %s to %s", from, to)
}

func (s PaymentState) CanTransitionTo(target PaymentState) bool {
	return ValidateTransition(s, target) == nil
}
