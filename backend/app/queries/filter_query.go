package queries

import (
	"database/sql"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

type FilterQueries struct {
	*sqlx.DB
}

func (q *FilterQueries) GetFilters() (*[]models.RegexFilter, error) {
	filters := &[]models.RegexFilter{}

	query := `SELECT * FROM regexfilters`

	if err := q.Select(filters, query); err != nil {
		return nil, err
	}

	return filters, nil
}

func (q *FilterQueries) GetFilter(id int64) (*models.RegexFilter, error) {
	filter := &models.RegexFilter{}

	query := `SELECT * FROM regexfilters WHERE id = ?`

	err := q.Get(filter, query, id)
	if err != nil {
		return nil, err
	}

	return filter, nil
}

func (q *FilterQueries) CreateFilter(filter *models.RegexFilter) (int64, error) {

	query := `INSERT INTO regexfilters VALUES (null, ?, ?)`

	res, err := q.Exec(query, filter.Name, filter.Regex)
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

func (q *FilterQueries) UpdateFilter(filter *models.RegexFilter) error {

	query := `UPDATE regexfilters SET name = ?, regex = ? WHERE id = ?`

	_, err := q.Exec(query, filter.Name, filter.Regex, filter.ID)
	if err != nil {
		return err
	}

	return nil
}

func (q *FilterQueries) DeleteFilter(id int64) error {

	query := `DELETE FROM regexfilters WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}

func (q *FilterQueries) GetGroupFilters(group_id int64) (*[]models.GroupFilter, error) {
	filters := &[]models.GroupFilter{}

	query := `SELECT * FROM groupfilters WHERE group_id = ?`

	if err := q.Select(filters, query, group_id); err != nil {
		return nil, err
	}

	return filters, nil
}

func (q *FilterQueries) CreateGroupFilter(filter *models.GroupFilter) (int64, error) {

	query := `INSERT INTO groupfilters VALUES (null, ?, ?, ?)`

	res, err := q.Exec(query, filter.GroupId, filter.FilterId, filter.Type)
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

func (q *FilterQueries) UpdateGroupFilter(filter *models.GroupFilter) error {

	query := `UPDATE groupfilters SET group_id = ?, filter_id = ?, type = ? WHERE id = ?`

	_, err := q.Exec(query, filter.GroupId, filter.FilterId, filter.Type, filter.ID)
	if err != nil {
		return err
	}

	return nil
}

func (q *FilterQueries) DeleteGroupFilter(id int64) error {

	query := `DELETE FROM groupfilters WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}
