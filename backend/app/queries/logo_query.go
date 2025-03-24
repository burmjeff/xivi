package queries

import (
	"context"
	"database/sql"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// SQL query constants
const (
	selectAllLogosQuery   = `SELECT * FROM logo`
	selectLogoByIdQuery   = `SELECT * FROM logo WHERE id = ?`
	selectLogoByNameQuery = `SELECT * FROM logo WHERE name = ?`
	insertLogoQuery       = `INSERT INTO logo VALUES (null, ?)`
	updateLogoQuery       = `UPDATE logo SET name = ? WHERE id = ?`
	deleteLogoQuery       = `DELETE FROM logo WHERE id = ?`
)

// LogoQueries struct for queries from Logo model.
type LogoQueries struct {
	BaseQueries
}

// NewLogoQueries creates a new LogoQueries instance
func NewLogoQueries(db *sqlx.DB) *LogoQueries {
	return &LogoQueries{
		BaseQueries: NewBaseQueries(db),
	}
}

// GetLogos retrieves all logos
func (q *LogoQueries) GetLogos(ctx context.Context) (*[]models.Logo, error) {
	logos := &[]models.Logo{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectAllLogosQuery)
		if err != nil {
			return err
		}

		return stmt.SelectContext(ctx, logos)
	})

	if err != nil {
		log.Error().Err(err).Msg("Error retrieving logos")
		return nil, err
	}

	return logos, nil
}

// GetLogo retrieves a logo by its ID
func (q *LogoQueries) GetLogo(ctx context.Context, id int64) (*models.Logo, error) {
	logo := &models.Logo{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectLogoByIdQuery)
		if err != nil {
			return err
		}

		return stmt.GetContext(ctx, logo, id)
	})

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		log.Error().Err(err).Int64("id", id).Msg("Error retrieving logo")
		return nil, err
	}

	return logo, nil
}

// GetLogoByName retrieves a logo by its name
func (q *LogoQueries) GetLogoByName(ctx context.Context, name string) (*models.Logo, error) {
	logo := &models.Logo{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectLogoByNameQuery)
		if err != nil {
			return err
		}

		return stmt.GetContext(ctx, logo, name)
	})

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		log.Error().Err(err).Str("name", name).Msg("Error retrieving logo by name")
		return nil, err
	}

	return logo, nil
}

// CreateLogo creates a new logo entry
func (q *LogoQueries) CreateLogo(ctx context.Context, name string) (int64, error) {
	var id int64

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(insertLogoQuery)
		if err != nil {
			return err
		}

		res, err := stmt.ExecContext(ctx, name)
		if err != nil {
			return err
		}

		id, err = res.LastInsertId()
		if err != nil {
			log.Warn().Err(err).Msg("Error retrieving the ID")
			return err
		}

		return nil
	})

	if err != nil {
		return 0, err
	}

	return id, nil
}

// UpdateLogo updates an existing logo entry
func (q *LogoQueries) UpdateLogo(ctx context.Context, id int64, p *models.Logo) error {
	return q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(updateLogoQuery)
		if err != nil {
			return err
		}

		_, err = stmt.ExecContext(ctx, p.Name, id)
		if err != nil {
			log.Error().Err(err).Int64("id", id).Msg("Error updating logo")
		}
		return err
	})
}

// DeleteLogo deletes a logo by its ID
func (q *LogoQueries) DeleteLogo(ctx context.Context, id int64) error {
	return q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(deleteLogoQuery)
		if err != nil {
			return err
		}

		_, err = stmt.ExecContext(ctx, id)
		if err != nil {
			log.Error().Err(err).Int64("id", id).Msg("Error deleting logo")
		}
		return err
	})
}
