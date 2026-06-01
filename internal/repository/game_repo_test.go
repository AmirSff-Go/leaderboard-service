package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
)

// newMock creates a sqlmock DB using the default regexp query matcher.
func newMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

// gameColumns are the columns returned by SELECT queries.
var gameColumns = []string{"id", "name", "description", "token_version", "created_at", "updated_at"}

// ---- GetByID ---------------------------------------------------------------------

func TestGetByID_Success(t *testing.T) {
	db, mock := newMock(t)
	repo := NewPostgresGameRepo(db)

	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	rows := sqlmock.NewRows(gameColumns).
		AddRow(id.String(), "My Game", "A great game", 1, now, now)

	mock.ExpectQuery(`WHERE id = \$1`).
		WithArgs(id.String()).
		WillReturnRows(rows)

	game, err := repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if game.ID != id {
		t.Errorf("ID: want %v, got %v", id, game.ID)
	}
	if game.Name != "My Game" {
		t.Errorf("Name: want %q, got %q", "My Game", game.Name)
	}
	if game.Description != "A great game" {
		t.Errorf("Description: want %q, got %q", "A great game", game.Description)
	}
	if game.TokenVersion != 1 {
		t.Errorf("TokenVersion: want 1, got %d", game.TokenVersion)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	db, mock := newMock(t)
	repo := NewPostgresGameRepo(db)

	id := uuid.New()

	mock.ExpectQuery(`WHERE id = \$1`).
		WithArgs(id.String()).
		WillReturnError(sql.ErrNoRows)

	_, err := repo.GetByID(context.Background(), id)
	if !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("want ErrGameNotFound, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetByID_DBError(t *testing.T) {
	db, mock := newMock(t)
	repo := NewPostgresGameRepo(db)

	id := uuid.New()
	dbErr := errors.New("connection reset")

	mock.ExpectQuery(`WHERE id = \$1`).
		WithArgs(id.String()).
		WillReturnError(dbErr)

	_, err := repo.GetByID(context.Background(), id)
	if !errors.Is(err, dbErr) {
		t.Fatalf("want %v, got %v", dbErr, err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// ---- Create ----------------------------------------------------------------------

func TestCreate_WithProvidedID(t *testing.T) {
	db, mock := newMock(t)
	repo := NewPostgresGameRepo(db)

	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	game := &domain.Game{ID: id, Name: "Game A", Description: "Desc A", TokenVersion: 1}

	rows := sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now)

	mock.ExpectQuery(`INSERT INTO games`).
		WithArgs(id, game.Name, game.Description, game.TokenVersion).
		WillReturnRows(rows)

	if err := repo.Create(context.Background(), game); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if game.ID != id {
		t.Errorf("ID should not change: want %v, got %v", id, game.ID)
	}
	if game.CreatedAt != now {
		t.Errorf("CreatedAt: want %v, got %v", now, game.CreatedAt)
	}
	if game.UpdatedAt != now {
		t.Errorf("UpdatedAt: want %v, got %v", now, game.UpdatedAt)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCreate_AutoGeneratesID(t *testing.T) {
	db, mock := newMock(t)
	repo := NewPostgresGameRepo(db)

	now := time.Now().UTC().Truncate(time.Second)
	game := &domain.Game{ID: uuid.Nil, Name: "Game B", Description: "Desc B", TokenVersion: 2}

	rows := sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now)

	// ID is auto-generated, so use AnyArg for the first argument.
	mock.ExpectQuery(`INSERT INTO games`).
		WithArgs(sqlmock.AnyArg(), game.Name, game.Description, game.TokenVersion).
		WillReturnRows(rows)

	if err := repo.Create(context.Background(), game); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if game.ID == uuid.Nil {
		t.Error("expected auto-generated UUID, got uuid.Nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCreate_DBError(t *testing.T) {
	db, mock := newMock(t)
	repo := NewPostgresGameRepo(db)

	dbErr := errors.New("duplicate key")
	game := &domain.Game{ID: uuid.New(), Name: "Game C", Description: "Desc C", TokenVersion: 1}

	mock.ExpectQuery(`INSERT INTO games`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(dbErr)

	if err := repo.Create(context.Background(), game); !errors.Is(err, dbErr) {
		t.Fatalf("want %v, got %v", dbErr, err)
	}
}

// ---- Update ----------------------------------------------------------------------

func TestUpdate_Success(t *testing.T) {
	db, mock := newMock(t)
	repo := NewPostgresGameRepo(db)

	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	game := &domain.Game{ID: id, Name: "Updated Name", Description: "Updated Desc", TokenVersion: 3}

	rows := sqlmock.NewRows([]string{"updated_at"}).AddRow(now)

	mock.ExpectQuery(`UPDATE games`).
		WithArgs(game.Name, game.Description, game.TokenVersion, id).
		WillReturnRows(rows)

	if err := repo.Update(context.Background(), game); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if game.UpdatedAt != now {
		t.Errorf("UpdatedAt: want %v, got %v", now, game.UpdatedAt)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	db, mock := newMock(t)
	repo := NewPostgresGameRepo(db)

	id := uuid.New()
	game := &domain.Game{ID: id, Name: "Gone", Description: "", TokenVersion: 1}

	mock.ExpectQuery(`UPDATE games`).
		WithArgs(game.Name, game.Description, game.TokenVersion, id).
		WillReturnError(sql.ErrNoRows)

	err := repo.Update(context.Background(), game)
	if !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("want ErrGameNotFound, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestUpdate_DBError(t *testing.T) {
	db, mock := newMock(t)
	repo := NewPostgresGameRepo(db)

	id := uuid.New()
	dbErr := errors.New("connection timeout")
	game := &domain.Game{ID: id, Name: "Game", Description: "D", TokenVersion: 1}

	mock.ExpectQuery(`UPDATE games`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(dbErr)

	if err := repo.Update(context.Background(), game); !errors.Is(err, dbErr) {
		t.Fatalf("want %v, got %v", dbErr, err)
	}
}
