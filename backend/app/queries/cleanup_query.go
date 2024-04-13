package queries

import (
	"github.com/jmoiron/sqlx"
)

type CleanupQueries struct {
	*sqlx.DB
}

func (q *CleanupQueries) VacuumDB() error {
	// Call INCREMENTAL VACUUM
	_, err := q.Exec("PRAGMA incremental_vacuum;")
	if err != nil {
		return err
	}
	return nil
}

func (q *CleanupQueries) CleanPlaylistGroups(id int64) error {
	query := `DELETE FROM playlistgroup 
		WHERE ? NOT IN (SELECT playlist_id FROM playlistgroup)`

	if _, err := q.Exec(query, id); err != nil {
		return err
	}

	return nil
}

func (q *CleanupQueries) CleanPlaylistChannels(id int64) error {
	query := `DELETE FROM playlistchannel
	WHERE ROWID IN (
		SELECT playlistchannel.ROWID FROM playlistchannel
		JOIN playlistgroup ON playlistchannel.group_id = playlistgroup.id
		WHERE ? NOT IN (SELECT playlist_id FROM playlistgroup)
		OR playlistgroup.enabled = false
	)`

	if _, err := q.Exec(query, id); err != nil {
		return err
	}

	return nil
}
