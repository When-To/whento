// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package models

// ListResponse represents a generic paginated list response.
type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// NewListResponse creates a new list response with items and total count.
func NewListResponse[T any](items []T, total int) *ListResponse[T] {
	if items == nil {
		items = []T{}
	}
	return &ListResponse[T]{
		Items: items,
		Total: total,
	}
}

// PagedListResponse represents a list response with pagination metadata.
type PagedListResponse[T any] struct {
	Items      []T `json:"items"`
	Total      int `json:"total"`
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalPages int `json:"total_pages"`
}

// NewPagedListResponse creates a new paged list response.
func NewPagedListResponse[T any](items []T, total, page, pageSize int) *PagedListResponse[T] {
	if items == nil {
		items = []T{}
	}
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	return &PagedListResponse[T]{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}
