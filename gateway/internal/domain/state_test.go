package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPaymentState_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		state PaymentState
		want  bool
	}{
		{"pending is valid", StatePending, true},
		{"authorized is valid", StateAuthorized, true},
		{"captured is valid", StateCaptured, true},
		{"voided is valid", StateVoided, true},
		{"refunded is valid", StateRefunded, true},
		{"failed is valid", StateFailed, true},
		{"invalid state", PaymentState("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.state.IsValid())
		})
	}
}

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    PaymentState
		to      PaymentState
		wantErr bool
	}{
		// Valid transitions
		{"pending to authorized", StatePending, StateAuthorized, false},
		{"pending to failed", StatePending, StateFailed, false},
		{"authorized to captured", StateAuthorized, StateCaptured, false},
		{"authorized to voided", StateAuthorized, StateVoided, false},
		{"authorized to failed", StateAuthorized, StateFailed, false},
		{"captured to refunded", StateCaptured, StateRefunded, false},
		{"captured to failed", StateCaptured, StateFailed, true},

		// Invalid transitions
		{"pending to captured", StatePending, StateCaptured, true},
		{"pending to voided", StatePending, StateVoided, true},
		{"pending to refunded", StatePending, StateRefunded, true},
		{"authorized to refunded", StateAuthorized, StateRefunded, true},
		{"captured to authorized", StateCaptured, StateAuthorized, true},
		{"captured to voided", StateCaptured, StateVoided, true},
		{"voided to any", StateVoided, StateAuthorized, true},
		{"refunded to any", StateRefunded, StateAuthorized, true},
		{"failed to any", StateFailed, StateAuthorized, true},

		// Invalid states
		{"invalid from state", PaymentState("invalid"), StateAuthorized, true},
		{"invalid to state", StatePending, PaymentState("invalid"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransition(tt.from, tt.to)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPaymentState_CanTransitionTo(t *testing.T) {
	tests := []struct {
		name   string
		from   PaymentState
		to     PaymentState
		expect bool
	}{
		{"pending can go to authorized", StatePending, StateAuthorized, true},
		{"pending cannot go to captured", StatePending, StateCaptured, false},
		{"authorized can go to captured", StateAuthorized, StateCaptured, true},
		{"authorized can go to voided", StateAuthorized, StateVoided, true},
		{"captured can go to refunded", StateCaptured, StateRefunded, true},
		{"voided cannot go anywhere", StateVoided, StateAuthorized, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, tt.from.CanTransitionTo(tt.to))
		})
	}
}
