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
	selectAllFiltersQuery = `SELECT * FROM regexfilters`
	selectFilterByIdQuery = `SELECT * FROM regexfilters WHERE id = ?`
	insertFilterQuery     = `INSERT INTO regexfilters VALUES (null, ?, ?)`
	updateFilterQuery     = `UPDATE regexfilters SET name = ?, regex = ? WHERE id = ?`
	deleteFilterQuery     = `DELETE FROM regexfilters WHERE id = ?`

	selectGroupFiltersByGroupIdQuery = `SELECT * FROM groupfilters WHERE group_id = ?`
	insertGroupFilterQuery           = `INSERT INTO groupfilters VALUES (null, ?, ?, ?)`
	updateGroupFilterQuery           = `UPDATE groupfilters SET group_id = ?, filter_id = ?, type = ? WHERE id = ?`
	deleteGroupFilterQuery           = `DELETE FROM groupfilters WHERE id = ?`
)

type FilterQueries struct {
	BaseQueries
}

// NewFilterQueries creates a new FilterQueries instance
func NewFilterQueries(db *sqlx.DB) *FilterQueries {
	return &FilterQueries{
		BaseQueries: NewBaseQueries(db),
	}
}

func (q *FilterQueries) GetFilters(ctx context.Context) (*[]models.RegexFilter, error) {
	filters := &[]models.RegexFilter{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectAllFiltersQuery)
		if err != nil {
			return err
		}

		return stmt.SelectContext(ctx, filters)
	})

	if err != nil {
		log.Error().Err(err).Msg("Error retrieving filters")
		return nil, err
	}

	return filters, nil
}

func (q *FilterQueries) GetFilter(ctx context.Context, id int64) (*models.RegexFilter, error) {
	filter := &models.RegexFilter{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectFilterByIdQuery)
		if err != nil {
			return err
		}

		return stmt.GetContext(ctx, filter, id)
	})

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		log.Error().Err(err).Int64("id", id).Msg("Error retrieving filter")
		return nil, err
	}

	return filter, nil
}

func (q *FilterQueries) CreateFilter(ctx context.Context, filter *models.RegexFilter) (int64, error) {
	var id int64

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(insertFilterQuery)
		if err != nil {
			return err
		}

		res, err := stmt.ExecContext(ctx, filter.Name, filter.Regex)
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

func (q *FilterQueries) UpdateFilter(ctx context.Context, filter *models.RegexFilter) error {
	return q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(updateFilterQuery)
		if err != nil {
			return err
		}

		_, err = stmt.ExecContext(ctx, filter.Name, filter.Regex, filter.ID)
		return err
	})
}

func (q *FilterQueries) DeleteFilter(ctx context.Context, id int64) error {
	return q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(deleteFilterQuery)
		if err != nil {
			return err
		}

		_, err = stmt.ExecContext(ctx, id)
		return err
	})
}

func (q *FilterQueries) GetGroupFilters(ctx context.Context, group_id int64) (*[]models.GroupFilter, error) {
	filters := &[]models.GroupFilter{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectGroupFiltersByGroupIdQuery)
		if err != nil {
			return err
		}

		return stmt.SelectContext(ctx, filters, group_id)
	})

	if err != nil {
		log.Error().Err(err).Int64("group_id", group_id).Msg("Error retrieving group filters")
		return nil, err
	}

	return filters, nil
}

func (q *FilterQueries) CreateGroupFilter(ctx context.Context, filter *models.GroupFilter) (int64, error) {
	var id int64

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(insertGroupFilterQuery)
		if err != nil {
			return err
		}

		res, err := stmt.ExecContext(ctx, filter.GroupId, filter.FilterId, filter.Type)
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

func (q *FilterQueries) UpdateGroupFilter(ctx context.Context, filter *models.GroupFilter) error {
	return q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(updateGroupFilterQuery)
		if err != nil {
			return err
		}

		_, err = stmt.ExecContext(ctx, filter.GroupId, filter.FilterId, filter.Type, filter.ID)
		return err
	})
}

func (q *FilterQueries) DeleteGroupFilter(ctx context.Context, id int64) error {
	return q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(deleteGroupFilterQuery)
		if err != nil {
			return err
		}

		_, err = stmt.ExecContext(ctx, id)
		return err
	})
}
