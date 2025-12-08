// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package models

import (
	"time"

	"github.com/google/uuid"
)

// Entity represents a base entity with a unique identifier.
type Entity struct {
	ID uuid.UUID `json:"id" db:"id"`
}

// TimestampedEntity represents an entity with creation and update timestamps.
type TimestampedEntity struct {
	Entity
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// SoftDeletableEntity represents an entity that can be soft deleted.
type SoftDeletableEntity struct {
	TimestampedEntity
	DeletedAt *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}
