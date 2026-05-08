package repository

import (
	"context"

	"gosocket/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type VideoRepo struct {
	db *pgxpool.Pool
}

func NewVideoRepo(db *pgxpool.Pool) *VideoRepo {
	return &VideoRepo{db: db}
}

func (r *VideoRepo) Create(ctx context.Context, title, filename string) (*models.Video, error) {
	v := &models.Video{}
	var status string
	err := r.db.QueryRow(ctx,
		`INSERT INTO videos (title, filename)
		 VALUES ($1, $2)
		 RETURNING id, title, filename, status, resolutions, duration, created_at, updated_at`,
		title, filename,
	).Scan(&v.ID, &v.Title, &v.Filename, &status, &v.Resolutions, &v.Duration, &v.CreatedAt, &v.UpdatedAt)
	v.Status = models.VideoStatus(status)
	return v, err
}

func (r *VideoRepo) GetByID(ctx context.Context, id string) (*models.Video, error) {
	v := &models.Video{}
	var status string
	err := r.db.QueryRow(ctx,
		`SELECT id, title, filename, status, resolutions, duration, created_at, updated_at
		 FROM videos WHERE id = $1`,
		id,
	).Scan(&v.ID, &v.Title, &v.Filename, &status, &v.Resolutions, &v.Duration, &v.CreatedAt, &v.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	v.Status = models.VideoStatus(status)
	return v, err
}

func (r *VideoRepo) SetStatus(ctx context.Context, id string, status models.VideoStatus) error {
	_, err := r.db.Exec(ctx,
		`UPDATE videos SET status = $1, updated_at = NOW() WHERE id = $2`,
		string(status), id,
	)
	return err
}

func (r *VideoRepo) SetReady(ctx context.Context, id string, resolutions []string, duration float64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE videos SET status = 'ready', resolutions = $1, duration = $2, updated_at = NOW() WHERE id = $3`,
		resolutions, duration, id,
	)
	return err
}
