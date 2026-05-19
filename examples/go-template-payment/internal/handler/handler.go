package handler

import (
	"context"

	"github.com/opencode-sig/runtime-sdk/examples/go-template-payment/internal/service"
	paymentv1 "github.com/opencode-sig/runtime-sdk/examples/go-template-payment/protobuf/payment/v1"
)

type Handler struct {
	paymentv1.UnimplementedPaymentServiceServer

	service *service.Service
}

func New(service *service.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetPayment(ctx context.Context, req *paymentv1.GetPaymentRequest) (*paymentv1.PaymentResponse, error) {
	payment, err := h.service.GetPayment(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toResponse(payment), nil
}

func (h *Handler) CreatePayment(ctx context.Context, req *paymentv1.CreatePaymentRequest) (*paymentv1.PaymentResponse, error) {
	payment, err := h.service.CreatePayment(ctx, req.GetOrderId(), req.GetAmount(), req.GetCurrency())
	if err != nil {
		return nil, err
	}
	return toResponse(payment), nil
}

func toResponse(payment service.Payment) *paymentv1.PaymentResponse {
	return &paymentv1.PaymentResponse{
		Id:       payment.ID,
		OrderId:  payment.OrderID,
		Amount:   payment.Amount,
		Currency: payment.Currency,
		Status:   payment.Status,
	}
}
