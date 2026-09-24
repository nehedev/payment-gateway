package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/nehemiah-dev/payment-gateway/internal/models"
)

func TestValidateAuthorizationRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     models.AuthorizePaymentRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request",
			req: models.AuthorizePaymentRequest{
				OrderID:    "order-123",
				CustomerID: "customer-456",
				Amount:     5000,
				Currency:   "USD",
				CardNumber: "4111111111111111",
				CardExpiry: "12/2030",
				CardCVV:    "123",
			},
			wantErr: false,
		},
		{
			name: "valid request with spaces in card number",
			req: models.AuthorizePaymentRequest{
				OrderID:    "order-123",
				CustomerID: "customer-456",
				Amount:     5000,
				Currency:   "USD",
				CardNumber: "4111 1111 1111 1111",
				CardExpiry: "12/2030",
				CardCVV:    "123",
			},
			wantErr: false,
		},
		{
			name: "invalid amount - zero",
			req: models.AuthorizePaymentRequest{
				OrderID:    "order-123",
				CustomerID: "customer-456",
				Amount:     0,
				Currency:   "USD",
				CardNumber: "4111111111111111",
				CardExpiry: "12/2030",
				CardCVV:    "123",
			},
			wantErr: true,
			errMsg:  "amount must be greater than 0",
		},
		{
			name: "invalid amount - negative",
			req: models.AuthorizePaymentRequest{
				OrderID:    "order-123",
				CustomerID: "customer-456",
				Amount:     -100,
				Currency:   "USD",
				CardNumber: "4111111111111111",
				CardExpiry: "12/2030",
				CardCVV:    "123",
			},
			wantErr: true,
			errMsg:  "amount must be greater than 0",
		},
		{
			name: "invalid currency - not 3 letters",
			req: models.AuthorizePaymentRequest{
				OrderID:    "order-123",
				CustomerID: "customer-456",
				Amount:     5000,
				Currency:   "US",
				CardNumber: "4111111111111111",
				CardExpiry: "12/2030",
				CardCVV:    "123",
			},
			wantErr: true,
			errMsg:  "currency must be a 3-letter ISO code",
		},
		{
			name: "invalid currency - lowercase",
			req: models.AuthorizePaymentRequest{
				OrderID:    "order-123",
				CustomerID: "customer-456",
				Amount:     5000,
				Currency:   "usd",
				CardNumber: "4111111111111111",
				CardExpiry: "12/2030",
				CardCVV:    "123",
			},
			wantErr: true,
			errMsg:  "currency must be a 3-letter ISO code",
		},
		{
			name: "invalid card number - too short",
			req: models.AuthorizePaymentRequest{
				OrderID:    "order-123",
				CustomerID: "customer-456",
				Amount:     5000,
				Currency:   "USD",
				CardNumber: "123456789012",
				CardExpiry: "12/2030",
				CardCVV:    "123",
			},
			wantErr: true,
			errMsg:  "card number must be 13-19 digits",
		},
		{
			name: "invalid card number - failed Luhn check",
			req: models.AuthorizePaymentRequest{
				OrderID:    "order-123",
				CustomerID: "customer-456",
				Amount:     5000,
				Currency:   "USD",
				CardNumber: "4111111111111112",
				CardExpiry: "12/2030",
				CardCVV:    "123",
			},
			wantErr: true,
			errMsg:  "invalid card number (failed Luhn check)",
		},
		{
			name: "invalid CVV - too short",
			req: models.AuthorizePaymentRequest{
				OrderID:    "order-123",
				CustomerID: "customer-456",
				Amount:     5000,
				Currency:   "USD",
				CardNumber: "4111111111111111",
				CardExpiry: "12/2030",
				CardCVV:    "12",
			},
			wantErr: true,
			errMsg:  "CVV must be 3-4 digits",
		},
		{
			name: "invalid CVV - too long",
			req: models.AuthorizePaymentRequest{
				OrderID:    "order-123",
				CustomerID: "customer-456",
				Amount:     5000,
				Currency:   "USD",
				CardNumber: "4111111111111111",
				CardExpiry: "12/2030",
				CardCVV:    "12345",
			},
			wantErr: true,
			errMsg:  "CVV must be 3-4 digits",
		},
		{
			name: "invalid expiry format",
			req: models.AuthorizePaymentRequest{
				OrderID:    "order-123",
				CustomerID: "customer-456",
				Amount:     5000,
				Currency:   "USD",
				CardNumber: "4111111111111111",
				CardExpiry: "12/30",
				CardCVV:    "123",
			},
			wantErr: true,
			errMsg:  "card expiry must be in format MM/YYYY",
		},
		{
			name: "empty order ID",
			req: models.AuthorizePaymentRequest{
				OrderID:    "",
				CustomerID: "customer-456",
				Amount:     5000,
				Currency:   "USD",
				CardNumber: "4111111111111111",
				CardExpiry: "12/2030",
				CardCVV:    "123",
			},
			wantErr: true,
			errMsg:  "order_id is required",
		},
		{
			name: "empty customer ID",
			req: models.AuthorizePaymentRequest{
				OrderID:    "order-123",
				CustomerID: "",
				Amount:     5000,
				Currency:   "USD",
				CardNumber: "4111111111111111",
				CardExpiry: "12/2030",
				CardCVV:    "123",
			},
			wantErr: true,
			errMsg:  "customer_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAuthorizationRequest(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLuhnCheck(t *testing.T) {
	tests := []struct {
		name       string
		cardNumber string
		want       bool
	}{
		{"valid Visa", "4111111111111111", true},
		{"valid Mastercard", "5555555555554444", true},
		{"valid Amex", "378282246310005", true},
		{"valid Discover", "6011111111111117", true},
		{"invalid - wrong checksum", "4111111111111112", false},
		{"invalid - all zeros", "0000000000000000", true}, // Luhn passes but would fail other validation
		{"invalid - random", "1234567890123456", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, luhnCheck(tt.cardNumber))
		})
	}
}

func TestParseCardExpiry(t *testing.T) {
	tests := []struct {
		name        string
		expiry      string
		wantMonth   int
		wantYear    int
		wantErr     bool
		errContains string
	}{
		{"valid expiry", "12/2030", 12, 2030, false, ""},
		{"valid expiry - January", "01/2025", 1, 2025, false, ""},
		{"valid expiry - single digit month with zero", "09/2026", 9, 2026, false, ""},
		{"invalid format - no slash", "122030", 0, 0, true, "expected format MM/YYYY"},
		{"invalid format - wrong separator", "12-2030", 0, 0, true, "expected format MM/YYYY"},
		{"invalid month - zero", "00/2030", 0, 0, true, "invalid month"},
		{"invalid month - too high", "13/2030", 0, 0, true, "invalid month"},
		{"invalid year - too low", "12/1999", 0, 0, true, "invalid year"},
		{"invalid year - too high", "12/2100", 0, 0, true, "invalid year"},
		{"invalid - letters", "ab/2030", 0, 0, true, "invalid month"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			month, year, err := parseCardExpiry(tt.expiry)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantMonth, month)
				assert.Equal(t, tt.wantYear, year)
			}
		})
	}
}

func TestExtractLastFourDigits(t *testing.T) {
	tests := []struct {
		name       string
		cardNumber string
		want       string
	}{
		{"16 digit card", "4111111111111111", "1111"},
		{"15 digit card", "378282246310005", "0005"},
		{"with spaces", "4111 1111 1111 1111", "1111"},
		{"with dashes", "4111-1111-1111-1111", "1111"},
		{"short number", "123", "123"},
		{"exactly 4 digits", "1234", "1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractLastFourDigits(tt.cardNumber))
		})
	}
}
