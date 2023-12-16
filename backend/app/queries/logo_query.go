package queries

import (
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// LogoQueries struct for queries from Logo model.
type LogoQueries struct {
	*sqlx.DB
}

// GetLogos method
func (q *LogoQueries) GetLogos() ([]models.Logo, error) {
	logos := []models.Logo{}

	// Define query string.
	query := `SELECT * FROM logo`

	// Send query to database.
	err := q.Select(&logos, query)
	if err != nil {
		// Return empty object and error.
		return logos, err
	}

	// Return query result.
	return logos, nil
}

// GetLogo method for getting one Logo by given ID.
func (q *LogoQueries) GetLogo(id int64) (models.Logo, error) {
	logo := models.Logo{}

	// Define query string.
	query := `SELECT * FROM logo WHERE id = ?`

	// Send query to database.
	err := q.Get(&logo, query, id)
	if err != nil {
		// Return empty object and error.
		return logo, err
	}

	// Return query result.
	return logo, nil
}

// CreateLogo method for creating a Logo by given Logo object.
func (q *LogoQueries) CreateLogo(name string) (int64, error) {
	// Define query string.
	query := `INSERT INTO logo VALUES (null, ?)`

	// Send query to database.
	res, err := q.Exec(query, name)
	if err != nil {
		// Return only error.
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return 0, err
	}

	// This query returns nothing.
	return id, nil
}

// UpdateLogo method for updating Logo by given Logo object.
func (q *LogoQueries) UpdateLogo(id int64, p *models.Logo) error {
	// Define query string.
	query := `UPDATE logo SET name = ? WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, p.Name, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// DeleteLogo method for delete logo by given ID.
func (q *LogoQueries) DeleteLogo(id int64) error {
	// Define query string.
	query := `DELETE FROM logo WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}
