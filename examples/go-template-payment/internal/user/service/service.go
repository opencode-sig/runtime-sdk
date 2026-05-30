package service

import (
	"context"
	"fmt"
	"strings"
)

type User struct {
	ID     string
	Name   string
	Email  string
	Status string
}

type Service struct{}

func New() *Service {
	return &Service{}
}

func (s *Service) GetUser(ctx context.Context, id string) (User, error) {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" {
		return User{}, fmt.Errorf("user id is required")
	}
	return User{
		ID:     id,
		Name:   "Runtime Demo User",
		Email:  "demo.user@example.com",
		Status: "active",
	}, nil
}
