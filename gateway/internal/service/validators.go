package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/nehemiah-dev/payment-gateway/internal/models"
)

var (
	cardNumberRegex = regexp.MustCompile(`^\d{13,19}$`)
	cvvRegex        = regexp.MustCompile(`^\d{3,4}$`)
	expiryRegex     = regexp.MustCompile(`^(0[1-9]|1[012])/\d{4}$`)
	currencyRegex   = regexp.MustCompile(`^[A-Z]{3}$`)
)

// ValidateAuthorizationRequest validates the authorization request.
func ValidateAuthorizationRequest(req *models.AuthorizePaymentRequest) error {
	// Validate amount
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}

	// Validate currency (3-letter ISO code)
	if !currencyRegex.MatchString(req.Currency) {
		return fmt.Errorf("currency must be a 3-letter ISO code (e.g., USD)")
	}

	// Validate card number (remove spaces/dashes first)
	cleanCard := strings.ReplaceAll(req.CardNumber, " ", "")
	cleanCard = strings.ReplaceAll(cleanCard, "-", "")
	if !cardNumberRegex.MatchString(cleanCard) {
		return fmt.Errorf("card number must be 13-19 digits")
	}

	// Validate Luhn algorithm for card number
	if !luhnCheck(cleanCard) {
		return fmt.Errorf("invalid card number (failed Luhn check)")
	}

	// Validate CVV
	if !cvvRegex.MatchString(req.CardCVV) {
		return fmt.Errorf("CVV must be 3-4 digits")
	}

	// Validate expiry format
	if !expiryRegex.MatchString(req.CardExpiry) {
		return fmt.Errorf("card expiry must be in format MM/YYYY")
	}

	// Validate order ID
	if strings.TrimSpace(req.OrderID) == "" {
		return fmt.Errorf("order_id is required")
	}

	// Validate customer ID
	if strings.TrimSpace(req.CustomerID) == "" {
		return fmt.Errorf("customer_id is required")
	}

	return nil
}

// ValidateCaptureRequest validates the capture request.
func ValidateCaptureRequest(req *models.CapturePaymentRequest) error {
	// Validate amount — the capture must move the FULL authorized amount; an
	// exact match against the authorized amount is checked against the stored
	// payment row in the service.
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}

	// Validate reference — it routes to the stored payment row.
	if _, err := uuid.Parse(req.Reference); err != nil {
		return fmt.Errorf("payment reference must be a valid UUID")
	}

	return nil
}

// ValidateRefundRequest validates the refund request.
func ValidateRefundRequest(req *models.RefundPaymentRequest) error {
	// Validate amount — the refund must move the FULL captured amount; an
	// exact match against the captured amount is checked against the stored
	// payment row in the service.
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}

	// Validate reference — it routes to the stored payment row.
	if _, err := uuid.Parse(req.Reference); err != nil {
		return fmt.Errorf("payment reference must be a valid UUID")
	}

	return nil
}

// ValidateVoidRequest validates the void request. The void is full-amount by
// definition; the authorization ID is taken from the stored payment row.
func ValidateVoidRequest(req *models.VoidPaymentRequest) error {
	// Validate reference — it routes to the stored payment row.
	if _, err := uuid.Parse(req.Reference); err != nil {
		return fmt.Errorf("payment reference must be a valid UUID")
	}

	return nil
}

// luhnCheck validates a card number using the Luhn algorithm.
func luhnCheck(cardNumber string) bool {
	var sum int
	parity := len(cardNumber) % 2

	for i, digit := range cardNumber {
		d := int(digit - '0')
		if i%2 == parity {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}

	return sum%10 == 0
}
