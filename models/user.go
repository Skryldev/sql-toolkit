package models

import "time"

// User represents a row in the "users" table.
// Fields map 1-to-1 with columns; no automatic relation loading.
type User struct {
	ID        int64
	Name      string
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateUserParams holds the fields required to create a new user.
// Keeping input types separate from the domain model prevents accidental
// mass-assignment and makes API contracts explicit.
type CreateUserParams struct {
	Name  string
	Email string
}

// UpdateUserParams holds fields that can be updated. All fields are pointers
// so callers only set what needs changing; the repository builds the explicit
// SQL accordingly.
type UpdateUserParams struct {
	ID    int64
	Name  *string
	Email *string
}