#!/bin/bash
set -e

echo "=== Testing /api/v1/payments/authorize endpoint ==="
echo ""

# Test 1: Successful authorization
echo "Test 1: Successful authorization with valid card"
curl -s -X POST http://localhost:8080/api/v1/payments/authorize \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: test-$(uuidgen)" \
  -d '{
    "order_id": "order-test-001",
    "customer_id": "customer-test-001",
    "amount": 5000,
    "currency": "USD",
    "card_number": "4111111111111111",
    "card_expiry": "12/2030",
    "card_cvv": "123"
  }' | jq .

echo ""
echo "---"
echo ""

# Test 2: Idempotency - replay same request
echo "Test 2: Idempotency test - should return cached response"
IDEMPOTENCY_KEY="test-idempotency-$(uuidgen)"
echo "Using idempotency key: $IDEMPOTENCY_KEY"

echo "First request:"
curl -s -X POST http://localhost:8080/api/v1/payments/authorize \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEMPOTENCY_KEY" \
  -d '{
    "order_id": "order-test-002",
    "customer_id": "customer-test-002",
    "amount": 3000,
    "currency": "USD",
    "card_number": "4111111111111111",
    "card_expiry": "12/2030",
    "card_cvv": "123"
  }' | jq .

echo ""
echo "Second request (should be replayed):"
curl -s -X POST http://localhost:8080/api/v1/payments/authorize \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEMPOTENCY_KEY" \
  -I | grep -E "(HTTP|X-Idempotent-Replayed)"

echo ""
echo "---"
echo ""

# Test 3: Missing idempotency key
echo "Test 3: Missing Idempotency-Key header (should fail)"
curl -s -X POST http://localhost:8080/api/v1/payments/authorize \
  -H "Content-Type: application/json" \
  -d '{
    "order_id": "order-test-003",
    "customer_id": "customer-test-003",
    "amount": 2000,
    "currency": "USD",
    "card_number": "4111111111111111",
    "card_expiry": "12/2030",
    "card_cvv": "123"
  }' | jq .

echo ""
echo "---"
echo ""

# Test 4: Invalid card number (Luhn check)
echo "Test 4: Invalid card number (should fail Luhn check)"
curl -s -X POST http://localhost:8080/api/v1/payments/authorize \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: test-$(uuidgen)" \
  -d '{
    "order_id": "order-test-004",
    "customer_id": "customer-test-004",
    "amount": 1000,
    "currency": "USD",
    "card_number": "4111111111111112",
    "card_expiry": "12/2030",
    "card_cvv": "123"
  }' | jq .

echo ""
echo "---"
echo ""

# Test 5: Insufficient funds
echo "Test 5: Card with insufficient funds (limited balance card)"
curl -s -X POST http://localhost:8080/api/v1/payments/authorize \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: test-$(uuidgen)" \
  -d '{
    "order_id": "order-test-005",
    "customer_id": "customer-test-005",
    "amount": 100000,
    "currency": "USD",
    "card_number": "4242424242424242",
    "card_expiry": "06/2030",
    "card_cvv": "456"
  }' | jq .

echo ""
echo "=== All tests completed ==="
