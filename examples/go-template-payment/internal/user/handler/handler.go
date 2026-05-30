package handler

import (
	"context"

	userservice "github.com/opencode-sig/runtime-sdk/examples/go-template-payment/internal/user/service"
	userv1 "github.com/opencode-sig/runtime-sdk/examples/go-template-payment/protobuf/user/v1"
)

type Handler struct {
	userv1.UnimplementedUserServiceServer

	service *userservice.Service
}

func New(service *userservice.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.UserResponse, error) {
	user, err := h.service.GetUser(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &userv1.UserResponse{
		Id:     user.ID,
		Name:   user.Name,
		Email:  user.Email,
		Status: user.Status,
	}, nil
}
