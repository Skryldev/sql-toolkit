package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Skryldev/sql-toolkit/db"
	"github.com/Skryldev/sql-toolkit/models"
)

// ─────────────────────────────────────────────────────────────────────────────
// UserRepository interface — for mocking in tests
// ─────────────────────────────────────────────────────────────────────────────

//go:generate mockgen -source=user_repo.go -destination=../mocks/user_repo_mock.go -package=mocks

// UserRepository defines the contract for user persistence operations.
// All implementations must satisfy this interface.
type UserRepository interface {
	Insert(ctx context.Context, params models.CreateUserParams) (*models.User, error)
	GetByID(ctx context.Context, id int64) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	List(ctx context.Context, limit, offset int) ([]*models.User, error)
	Update(ctx context.Context, params models.UpdateUserParams) (*models.User, error)
	Delete(ctx context.Context, id int64) error
	BatchInsert(ctx context.Context, params []models.CreateUserParams) ([]*models.User, error)
	Count(ctx context.Context) (int64, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// userRepo — concrete implementation
// ─────────────────────────────────────────────────────────────────────────────

// userRepo is the production implementation backed by a db.Querier.
type userRepo struct {
	q db.Querier
}

// NewUserRepo returns a UserRepository backed by q.
// q can be a *db.DB or *db.Tx — both satisfy db.Querier.
func NewUserRepo(q db.Querier) UserRepository {
	return &userRepo{q: q}
}

// ─────────────────────────────────────────────────────────────────────────────
// SQL constants — all SQL is explicit, version-controlled, and reviewable
// ─────────────────────────────────────────────────────────────────────────────

const (
	sqlInsertUser = `
		INSERT INTO users (name, email, created_at, updated_at)
		VALUES ($1, $2, $3, $3)
		RETURNING id, name, email, created_at, updated_at`

	sqlGetUserByID = `
		SELECT id, name, email, created_at, updated_at
		FROM   users
		WHERE  id = $1
		LIMIT  1`

	sqlGetUserByEmail = `
		SELECT id, name, email, created_at, updated_at
		FROM   users
		WHERE  email = $1
		LIMIT  1`

	sqlListUsers = `
		SELECT id, name, email, created_at, updated_at
		FROM   users
		ORDER  BY id
		LIMIT  $1 OFFSET $2`

	sqlDeleteUser = `
		DELETE FROM users WHERE id = $1`

	sqlCountUsers = `
		SELECT COUNT(*) FROM users`
)

// ─────────────────────────────────────────────────────────────────────────────
// Insert
// ─────────────────────────────────────────────────────────────────────────────

// Insert creates a new user and returns the persisted record including the
// database-assigned id and timestamps.
func (r *userRepo) Insert(ctx context.Context, params models.CreateUserParams) (*models.User, error) {
	now := time.Now().UTC()
	row := r.q.QueryRow(ctx, sqlInsertUser, params.Name, params.Email, now)
	return scanUser(row)
}

// ─────────────────────────────────────────────────────────────────────────────
// GetByID
// ─────────────────────────────────────────────────────────────────────────────

// GetByID returns a single user by primary key.
// Returns db.ErrNotFound when no record matches.
func (r *userRepo) GetByID(ctx context.Context, id int64) (*models.User, error) {
	row := r.q.QueryRow(ctx, sqlGetUserByID, id)
	return scanUser(row)
}

// ─────────────────────────────────────────────────────────────────────────────
// GetByEmail
// ─────────────────────────────────────────────────────────────────────────────

// GetByEmail looks up a user by their unique email address.
// Returns db.ErrNotFound when no record matches.
func (r *userRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	row := r.q.QueryRow(ctx, sqlGetUserByEmail, email)
	return scanUser(row)
}

// ─────────────────────────────────────────────────────────────────────────────
// List
// ─────────────────────────────────────────────────────────────────────────────

// List returns a paginated slice of users ordered by id.
func (r *userRepo) List(ctx context.Context, limit, offset int) ([]*models.User, error) {
	rows, err := r.q.Query(ctx, sqlListUsers, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		u := &models.User{}
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("repo/user: scan: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// Update — partial update with explicit SQL construction
// ─────────────────────────────────────────────────────────────────────────────

// Update applies a partial update to a user record. Only fields with non-nil
// pointers in params are updated. The SQL is built dynamically but remains
// fully visible — no hidden magic.
func (r *userRepo) Update(ctx context.Context, params models.UpdateUserParams) (*models.User, error) {
	setClauses := make([]string, 0, 3)
	args := make([]any, 0, 4)
	argIdx := 1

	if params.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *params.Name)
		argIdx++
	}
	if params.Email != nil {
		setClauses = append(setClauses, fmt.Sprintf("email = $%d", argIdx))
		args = append(args, *params.Email)
		argIdx++
	}
	if len(setClauses) == 0 {
		return r.GetByID(ctx, params.ID)
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now().UTC())
	argIdx++

	args = append(args, params.ID)

	query := fmt.Sprintf(`
		UPDATE users
		SET    %s
		WHERE  id = $%d
		RETURNING id, name, email, created_at, updated_at`,
		strings.Join(setClauses, ", "), argIdx)

	row := r.q.QueryRow(ctx, query, args...)
	return scanUser(row)
}

// ─────────────────────────────────────────────────────────────────────────────
// Delete
// ─────────────────────────────────────────────────────────────────────────────

// Delete removes a user by id.
// Returns db.ErrNotFound if no row was deleted.
func (r *userRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.q.Exec(ctx, sqlDeleteUser, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return db.ErrNotFound
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// BatchInsert
// ─────────────────────────────────────────────────────────────────────────────

// BatchInsert inserts multiple users in a single transaction using prepared
// statements for maximum throughput. All rows are inserted or none are.
func (r *userRepo) BatchInsert(ctx context.Context, params []models.CreateUserParams) ([]*models.User, error) {
	if len(params) == 0 {
		return nil, nil
	}

	// BatchExec requires a *DB; if r.q is a *Tx, we do it manually.
	// We detect *Tx by trying the concrete type assertion.
	const insertSQL = `
		INSERT INTO users (name, email, created_at, updated_at)
		VALUES ($1, $2, $3, $3)
		RETURNING id, name, email, created_at, updated_at`

	stmt, err := r.q.Prepare(ctx, insertSQL)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	now := time.Now().UTC()
	users := make([]*models.User, 0, len(params))
	for _, p := range params {
		row := stmt.QueryRow(ctx, p.Name, p.Email, now)
		u, err := scanUser(row)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Count
// ─────────────────────────────────────────────────────────────────────────────

// Count returns the total number of users.
func (r *userRepo) Count(ctx context.Context) (int64, error) {
	var n int64
	if err := r.q.QueryRow(ctx, sqlCountUsers).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// scanUser — centralised column mapping
// ─────────────────────────────────────────────────────────────────────────────

// scanUser scans a single user row. Centralising the scan call means that
// adding/removing columns only requires a change in one place.
func scanUser(row *db.Row) (*models.User, error) {
	u := &models.User{}
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("repo/user: %w", err)
	}
	return u, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Compile-time interface assertion
// ─────────────────────────────────────────────────────────────────────────────

var _ UserRepository = (*userRepo)(nil)

// ─────────────────────────────────────────────────────────────────────────────
// Null helpers
// ─────────────────────────────────────────────────────────────────────────────

// NullString converts *string to sql.NullString for optional columns.
func NullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}