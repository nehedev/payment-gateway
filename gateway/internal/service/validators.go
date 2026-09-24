package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nehemiah-dev/payment-gateway/internal/models"
)

var (
	cardNumberRegex = regexp.MustCompile(`^[0-9]{13,19}$`)
	cvvRegex        = regexp.MustCompile(`^[0-9]{3,4}$`)
	expiryRegex     = regexp.MustCompile(`^(0[1-9]|1[0-2])/[0-9]{4}$`)
	currencyRegex   = regexp.MustCompile(`^[A-Z]{3}$`)
)

// ValidateAuthorizationRequest validates all fields of an authorization request
// before it touches the bank. Structural checks (format, Luhn) are done here
// so invalid cards are rejected locally without a network round-trip.
func ValidateAuthorizationRequest(req models.AuthorizePaymentRequest) error {
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}

	if !currencyRegex.MatchString(req.Currency) {
		return fmt.Errorf("currency must be a 3-letter ISO code (e.g., USD)")
	}

	// Strip presentation formatting before digit-only checks.
	cleanCard := strings.ReplaceAll(req.CardNumber, " ", "")
	cleanCard = strings.ReplaceAll(cleanCard, "-", "")
	if !cardNumberRegex.MatchString(cleanCard) {
		return fmt.Errorf("card number must be 13-19 digits")
	}

	if !luhnCheck(cleanCard) {
		return fmt.Errorf("invalid card number (failed Luhn check)")
	}

	if !cvvRegex.MatchString(req.CardCVV) {
		return fmt.Errorf("CVV must be 3-4 digits")
	}

	if !expiryRegex.MatchString(req.CardExpiry) {
		return fmt.Errorf("card expiry must be in format MM/YYYY")
	}

	if strings.TrimSpace(req.OrderID) == "" {
		return fmt.Errorf("order_id is required")
	}

	if strings.TrimSpace(req.CustomerID) == "" {
		return fmt.Errorf("customer_id is required")
	}

	return nil
}

// luhnCheck returns true when cardNumber passes the Luhn checksum algorithm.
// It expects a digit-only string (no spaces or dashes).
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
