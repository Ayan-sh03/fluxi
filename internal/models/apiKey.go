package models

import (
	"time"

	"github.com/google/uuid"
)

type APIKey struct {
	ID         uuid.UUID `json:"id"`
	KeyValue   string    `json:"value"`
	UserID     uuid.UUID `json:"user_id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
	IsActive   bool      `json:"is_active"`
}

func NewAPIKey(userID uuid.UUID, name string) *APIKey {
	return &APIKey{
		ID:        uuid.New(),
		KeyValue:  uuid.New().String(),
		UserID:    userID,
		Name:      name,
		CreatedAt: time.Now(),
		IsActive:  true,
	}
}
