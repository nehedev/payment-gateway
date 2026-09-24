package service

import (
	"github.com/nehemiah-dev/payment-gateway/internal/bankclient"
	"github.com/nehemiah-dev/payment-gateway/internal/database"
	"github.com/nehemiah-dev/payment-gateway/internal/repository"
)

// PaymentService provides payment processing operations backed by a bank gateway.
type PaymentService struct {
	db         *database.DB
	repo       *repository.Repository
	bankClient bankclient.Gateway
}

func NewPaymentService(db *database.DB, repo *repository.Repository, bankClient bankclient.Gateway) *PaymentService {
	return &PaymentService{
		db:         db,
		repo:       repo,
		bankClient: bankClient,
	}
}
