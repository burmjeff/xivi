package queries

import (
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
)

// EpgQueries struct for queries from Epg model.
type VectorQueries struct {
	*sqlx.DB
}

// GetTemplateChannelVector method
func (q *VectorQueries) GetTemplateChannelVector(name string) (models.TemplateChannelVector, error) {
	vectorChannel := models.TemplateChannelVector{}

	// Define query string.
	query := `SELECT * FROM templatechannelvectors WHERE name = ?`

	// Send query to database.
	err := q.Select(&vectorChannel, query, name)
	if err != nil {
		// Return empty object and error.
		return vectorChannel, err
	}

	// Return query result.
	return vectorChannel, nil
}

// CreateTemplateChannelVector method
func (q *VectorQueries) CreateTemplateChannelVector(channelVector models.TemplateChannelVector) error {

	// Define query string.
	query := `INSERT INTO templatechannelvectors VALUES (null, ?, ?)`
	// Send query to database.
	_, err := q.Exec(query, channelVector.Name, channelVector.ChannelId)
	if err != nil {
		// Return empty object and error.
		return err
	}

	// Return query result.
	return nil
}

// GetChannelVector method
func (q *VectorQueries) GetChannelVector(name string) (models.ChannelVector, error) {
	vectorChannel := models.ChannelVector{}

	// Define query string.
	query := `SELECT * FROM channelvectors WHERE name = ?`

	// Send query to database.
	err := q.Get(&vectorChannel, query, name)
	if err != nil {
		// Return empty object and error.
		return vectorChannel, err
	}

	// Return query result.
	return vectorChannel, nil
}

// CreateChannelVector method
func (q *VectorQueries) CreateChannelVector(channelVector models.ChannelVector) error {

	// Define query string.
	query := `INSERT INTO channelvectors VALUES (null, ?, ?)`
	// Send query to database.
	_, err := q.Exec(query, channelVector.Name, channelVector.Vector)
	if err != nil {
		// Return empty object and error.
		return err
	}

	// Return query result.
	return nil
}

// GetChannelVector method
func (q *VectorQueries) GetFilter(oldName string) (models.ChannelFilter, error) {
	filter := models.ChannelFilter{}

	// Define query string.
	query := `SELECT * FROM channelfilters WHERE oldname = ?`

	// Send query to database.
	err := q.Get(&filter, query, oldName)
	if err != nil {
		// Return empty object and error.
		return filter, err
	}

	// Return query result.
	return filter, nil
}

// CreateFilter method
func (q *VectorQueries) CreateFilter(channelFilter models.ChannelFilter) error {

	// Define query string.
	query := `INSERT INTO channelfilters VALUES (null, ?, ?)`
	// Send query to database.
	_, err := q.Exec(query, channelFilter.OldName, channelFilter.NewName)
	if err != nil {
		// Return empty object and error.
		return err
	}

	// Return query result.
	return nil
}
