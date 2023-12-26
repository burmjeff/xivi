package queries

import (
	"database/sql"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// LogoQueries struct for queries from Logo model.
type LogoQueries struct {
	*sqlx.DB
}

// GetLogos
func (q *LogoQueries) GetLogos() (*[]models.Logo, error) {
	logos := &[]models.Logo{}

	query := `SELECT * FROM logo`

	err := q.Select(logos, query)
	if err != nil {
		return nil, err
	}

	return logos, nil
}

// Get a Logo by given ID.
func (q *LogoQueries) GetLogo(id int64) (*models.Logo, error) {
	logo := &models.Logo{}

	query := `SELECT * FROM logo WHERE id = ?`

	err := q.Get(logo, query, id)
	if err != nil {
		return nil, err
	}

	return logo, nil
}

// Get a Logo by name.
func (q *LogoQueries) GetLogoByName(name string) (*models.Logo, error) {
	logo := &models.Logo{}

	query := `SELECT * FROM logo WHERE name = ?`

	err := q.Get(logo, query, name)
	if err != nil {
		return nil, err
	}

	return logo, nil
}

// Create a Logo by given Logo object.
func (q *LogoQueries) CreateLogo(name string) (int64, error) {
	query := `INSERT INTO logo VALUES (null, ?)`

	res, err := q.Exec(query, name)
	if err != sql.ErrNoRows && err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return 0, err
	}

	return id, nil
}

// Update a Logo by given Logo object.
func (q *LogoQueries) UpdateLogo(id int64, p *models.Logo) error {
	query := `UPDATE logo SET name = ? WHERE id = ?`

	_, err := q.Exec(query, p.Name, id)
	if err != nil {
		return err
	}

	return nil
}

// Delete a logo by given ID.
func (q *LogoQueries) DeleteLogo(id int64) error {
	query := `DELETE FROM logo WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}
