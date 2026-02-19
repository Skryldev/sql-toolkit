package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/Skryldev/sql-toolkit/db"
	"github.com/Skryldev/sql-toolkit/models"
	"github.com/Skryldev/sql-toolkit/repo"
	_ "github.com/mattn/go-sqlite3"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test fixture
// ─────────────────────────────────────────────────────────────────────────────

func newTestRepo(t *testing.T) (repo.UserRepository, *db.DB) {
	t.Helper()

	database, err := db.Open(db.Config{
		DSN:        ":memory:",
		DriverName: "sqlite3",
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	_, err = database.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL,
			email      TEXT NOT NULL UNIQUE,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}

	return repo.NewUserRepo(database), database
}

// ─────────────────────────────────────────────────────────────────────────────
// Insert
// ─────────────────────────────────────────────────────────────────────────────

func TestUserRepo_Insert(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	u, err := r.Insert(ctx, models.CreateUserParams{
		Name:  "Alice",
		Email: "alice@repo.com",
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if u.Name != "Alice" {
		t.Fatalf("unexpected name: %q", u.Name)
	}
	if u.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt")
	}
}

func TestUserRepo_Insert_DuplicateEmail(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	params := models.CreateUserParams{Name: "Alice", Email: "dup@repo.com"}
	if _, err := r.Insert(ctx, params); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	_, err := r.Insert(ctx, params)
	if !db.IsDuplicateKey(err) {
		t.Fatalf("expected ErrDuplicateKey, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GetByID
// ─────────────────────────────────────────────────────────────────────────────

func TestUserRepo_GetByID(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	created, _ := r.Insert(ctx, models.CreateUserParams{Name: "Bob", Email: "bob@repo.com"})

	fetched, err := r.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if fetched.Email != "bob@repo.com" {
		t.Fatalf("wrong email: %q", fetched.Email)
	}
}

func TestUserRepo_GetByID_NotFound(t *testing.T) {
	r, _ := newTestRepo(t)
	_, err := r.GetByID(context.Background(), 99999)
	if !db.IsNotFound(err) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Update
// ─────────────────────────────────────────────────────────────────────────────

func TestUserRepo_Update_Name(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	u, _ := r.Insert(ctx, models.CreateUserParams{Name: "Old Name", Email: "upd@repo.com"})

	newName := "New Name"
	updated, err := r.Update(ctx, models.UpdateUserParams{ID: u.ID, Name: &newName})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "New Name" {
		t.Fatalf("unexpected name: %q", updated.Name)
	}
	if updated.Email != "upd@repo.com" {
		t.Fatalf("email should be unchanged: %q", updated.Email)
	}
}

func TestUserRepo_Update_NilFields_NoChange(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	u, _ := r.Insert(ctx, models.CreateUserParams{Name: "Static", Email: "static@repo.com"})

	// No fields to update — should return current record unchanged.
	same, err := r.Update(ctx, models.UpdateUserParams{ID: u.ID})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if same.Name != "Static" {
		t.Fatalf("unexpected change: %q", same.Name)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Delete
// ─────────────────────────────────────────────────────────────────────────────

func TestUserRepo_Delete(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	u, _ := r.Insert(ctx, models.CreateUserParams{Name: "Del", Email: "del@repo.com"})

	if err := r.Delete(ctx, u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := r.GetByID(ctx, u.ID)
	if !db.IsNotFound(err) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

func TestUserRepo_Delete_NotFound(t *testing.T) {
	r, _ := newTestRepo(t)
	err := r.Delete(context.Background(), 99999)
	if !db.IsNotFound(err) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// List
// ─────────────────────────────────────────────────────────────────────────────

func TestUserRepo_List(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	for i := range 5 {
		_, err := r.Insert(ctx, models.CreateUserParams{
			Name:  "User",
			Email: "list" + string(rune('0'+i)) + "@repo.com",
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	page, err := r.List(ctx, 3, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page) != 3 {
		t.Fatalf("expected 3, got %d", len(page))
	}

	page2, _ := r.List(ctx, 3, 3)
	if len(page2) != 2 {
		t.Fatalf("expected 2 on page 2, got %d", len(page2))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// BatchInsert
// ─────────────────────────────────────────────────────────────────────────────

func TestUserRepo_BatchInsert(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	params := []models.CreateUserParams{
		{Name: "Batch A", Email: "ba@repo.com"},
		{Name: "Batch B", Email: "bb@repo.com"},
		{Name: "Batch C", Email: "bc@repo.com"},
	}
	inserted, err := r.BatchInsert(ctx, params)
	if err != nil {
		t.Fatalf("batch insert: %v", err)
	}
	if len(inserted) != 3 {
		t.Fatalf("expected 3, got %d", len(inserted))
	}
	for _, u := range inserted {
		if u.ID == 0 {
			t.Fatal("expected non-zero ID in batch result")
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Transaction: repo inside tx
// ─────────────────────────────────────────────────────────────────────────────

func TestUserRepo_InsideTransaction(t *testing.T) {
	_, database := newTestRepo(t)
	ctx := context.Background()

	var createdID int64

	err := database.ExecTx(ctx, func(tx *db.Tx) error {
		txRepo := repo.NewUserRepo(tx)
		u, err := txRepo.Insert(ctx, models.CreateUserParams{
			Name:  "TxUser",
			Email: "tx@repo.com",
		})
		if err != nil {
			return err
		}
		createdID = u.ID
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}

	// Verify row is visible after commit
	r := repo.NewUserRepo(database)
	u, err := r.GetByID(ctx, createdID)
	if err != nil {
		t.Fatalf("post-tx get: %v", err)
	}
	if u.Email != "tx@repo.com" {
		t.Fatalf("unexpected email: %q", u.Email)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Count
// ─────────────────────────────────────────────────────────────────────────────

func TestUserRepo_Count(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	n, err := r.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}

	for i := range 4 {
		_ = time.Second // use time import
		_, _ = r.Insert(ctx, models.CreateUserParams{
			Name:  "U",
			Email: "cnt" + string(rune('a'+i)) + "@repo.com",
		})
	}

	n, _ = r.Count(ctx)
	if n != 4 {
		t.Fatalf("expected 4, got %d", n)
	}
}