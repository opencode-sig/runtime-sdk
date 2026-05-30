package service

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
)

type Payment struct {
	ID       string
	OrderID  string
	Amount   int64
	Currency string
	Status   string
}

type Settings struct {
	Status string `json:"status" yaml:"status"`
}

type UserProbe interface {
	CheckUser(ctx context.Context) error
}

type Service struct {
	status    atomic.Value
	userProbe UserProbe
}

func New() *Service {
	s := &Service{}
	s.status.Store("created")
	return s
}

func (s *Service) ApplySettings(settings Settings) {
	status := strings.TrimSpace(settings.Status)
	if status == "" {
		status = "created"
	}
	s.status.Store(status)
}

func (s *Service) SetUserProbe(probe UserProbe) {
	s.userProbe = probe
}

func (s *Service) GetPayment(ctx context.Context, id string) (Payment, error) {
	if strings.TrimSpace(id) == "" {
		return Payment{}, fmt.Errorf("payment id is required")
	}
	if s.userProbe != nil {
		if err := s.userProbe.CheckUser(ctx); err != nil {
			return Payment{}, fmt.Errorf("check user service connection: %w", err)
		}
	}
	return Payment{
		ID:       id,
		OrderID:  "order-1001",
		Amount:   9900,
		Currency: "CNY",
		Status:   s.currentStatus(),
	}, nil
}

func (s *Service) CreatePayment(ctx context.Context, orderID string, amount int64, currency string) (Payment, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return Payment{}, fmt.Errorf("order id is required")
	}
	if amount <= 0 {
		return Payment{}, fmt.Errorf("amount must be positive")
	}
	currency = strings.TrimSpace(currency)
	if currency == "" {
		currency = "CNY"
	}
	return Payment{
		ID:       "pay-1001",
		OrderID:  orderID,
		Amount:   amount,
		Currency: currency,
		Status:   s.currentStatus(),
	}, nil
}

func (s *Service) currentStatus() string {
	value, _ := s.status.Load().(string)
	if value == "" {
		return "created"
	}
	return value
}
