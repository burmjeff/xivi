package queries

import (
	"xivi/backend/app/models"
	utils "xivi/backend/pkg/dbutils"

	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
)

// EpgQueries struct for queries from Epg model.
type EpgQueries struct {
	*sqlx.DB
}

// GetEpgs method
func (q *EpgQueries) GetEpgs() ([]models.Epg, error) {
	epgs := []models.Epg{}

	// Define query string.
	query := `SELECT * FROM epg`

	// Send query to database.
	err := q.Select(&epgs, query)
	if err != nil {
		// Return empty object and error.
		return nil, err
	}

	// Return query result.
	return epgs, nil
}

// GetEpg method for getting one epg by given ID.
func (q *EpgQueries) GetEpg(id int64) (models.Epg, error) {
	epg := models.Epg{}

	// Define query string.
	query := `SELECT * FROM epg WHERE id = $1`

	// Send query to database.
	err := q.Get(&epg, query, id)
	if err != nil {
		// Return empty object and error.
		return epg, err
	}

	// Return query result.
	return epg, nil
}

// CreateEpg method for creating a epg by given Epg object.
func (q *EpgQueries) CreateEpg(p *models.Epg) (int64, error) {
	// Define query string.
	query := `INSERT INTO epg VALUES (null, $1, $2)`

	// Send query to database.
	res, err := q.Exec(query, p.Name, p.URL)
	if err != nil {
		// Return only error.
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Warnln("Error retrieving the ID: %v", err)
		return 0, err
	}

	// This query returns nothing.
	return id, nil
}

// UpdateEpg method for updating epg by given Epg object.
func (q *EpgQueries) UpdateEpg(id int64, p *models.Epg) error {
	// Define query string.
	query := `UPDATE epg SET name = $2, url = $3 WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id, p.Name, p.URL)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// DeleteEpg method for delete epg by given ID.
func (q *EpgQueries) DeleteEpg(id int64) error {
	// Define query string.
	query := `DELETE FROM epg WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// Get Epg Channels method
func (q *EpgQueries) GetEpgChannels() ([]models.EpgChannel, error) {
	epgchannel := []models.EpgChannel{}

	// Define query string.
	query := `SELECT * FROM epgchannel`

	// Send query to database.
	err := q.Select(&epgchannel, query)
	if err != nil {
		// Return empty object and error.
		return epgchannel, err
	}

	// Return query result.
	return epgchannel, nil
}

// Get Epg Channel method for getting one goup by given ID.
func (q *EpgQueries) GetEpgChannel(id int64) (models.EpgChannel, error) {
	// Define epg variable.
	epgchannel := models.EpgChannel{}

	// Define query string.
	query := `SELECT * FROM epgchannel WHERE id = $1`

	// Send query to database.
	err := q.Get(&epgchannel, query, id)
	if err != nil {
		// Return empty object and error.
		return epgchannel, err
	}

	// Return query result.
	return epgchannel, nil
}

// Get Epg Channel method for getting one epgchannel by given Name.
func (q *EpgQueries) GetEpgChannelByChannelId(channelID string) (models.EpgChannel, error) {
	// Define epgchannel variable.
	epgchannel := models.EpgChannel{}

	// Define query string.
	query := `SELECT * FROM epgchannel WHERE channelid = $1`

	// Send query to database.
	err := q.Get(&epgchannel, query, channelID)
	if err != nil {
		// Return empty object and error.
		return epgchannel, err
	}

	// Return query result.
	return epgchannel, nil
}

// Create Epg Channel method for creating epgchannel by given Epg Channel object.
func (q *EpgQueries) CreateEpgChannel(p *models.EpgChannel) (int64, error) {
	// Define query string.
	query := `INSERT INTO epgchannel VALUES (null, $1)`

	// Send query to database.
	res, err := q.Exec(query, p.Name)
	if err != nil {
		// Return only error.
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Warnln("Error retrieving the ID: %v", err)
		return 0, err
	}

	// This query returns nothing.
	return id, nil
}

// Update Epg Channel method for updating epgchannel by given Epg Channel object.
func (q *EpgQueries) UpdateEpgChannel(id int64, p *models.EpgChannel) error {
	// Define query string.
	query := `UPDATE epgchannel SET name = $2 WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id, p.Name)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// Delete Epg Channel method for delete Epg Channel by given ID.
func (q *EpgQueries) DeleteEpgChannel(id int64) error {
	// Define query string.
	query := `DELETE FROM epgchannel WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// Get Epg Programmes method for getting all programmes.
func (q *EpgQueries) GetEpgProgrammes() ([]models.EpgProgramme, error) {
	programmes := []models.EpgProgramme{}

	// Define query string.
	query := `SELECT * FROM epgprogramme`

	// Send query to database.
	err := q.Select(&programmes, query)
	if err != nil {
		// Return empty object and error.
		return programmes, err
	}

	// Return query result.
	return programmes, nil
}

// Get Epg Programmes method for getting all programmes by channel.
func (q *EpgQueries) GetProgrammesByChannel(id int64) ([]models.EpgProgramme, error) {
	programmes := []models.EpgProgramme{}

	// Define query string.
	query := `SELECT DISTINCT epgprogramme.id
	FROM epgchannel
	JOIN epgchannelitem ON epgchannel.id = epgchannelitem.epg_channel_id
	JOIN epgprogramme ON epgchannelitem.epg_programme_id = epgprogramme.id 
	WHERE epgchannel.id = $1`

	// Send query to database.
	err := q.Select(&programmes, query, id)
	if err != nil {
		// Return empty object and error.
		return programmes, err
	}

	// Return query result.
	return programmes, nil
}

// Get Epg Programme method for getting one programme by given ID.
func (q *EpgQueries) GetEpgProgramme(id int64) (models.EpgProgramme, error) {
	// Define programme variable.
	programme := models.EpgProgramme{}

	// Define query string.
	query := `SELECT * FROM epgprogramme WHERE id = $1`

	// Send query to database.
	err := q.Select(&programme, query, id)
	if err != nil {
		// Return empty object and error.
		return programme, err
	}

	// Return query result.
	return programme, nil
}

// Create Epg Programme method for creating a programme by given object.
func (q *EpgQueries) CreateEpgProgramme(p *models.EpgProgramme) (int64, error) {
	// Define query string.
	query := `INSERT INTO epgprogramme VALUES (null, $1, $2, $3, $4, $5, $6, $7, $8, 
		$9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`

	// Send query to database.
	res, err := q.Exec(query, p.Start, p.Stop, utils.NewNullString(p.Channel),
		utils.NewNullString(p.Title), utils.NewNullString(p.Subtitle),
		utils.NewNullString(p.Desc), utils.NewNullString(p.Credits),
		utils.NewNullString(p.Date), utils.NewNullString(p.Category_1),
		utils.NewNullString(p.Category_1), utils.NewNullString(p.Category_1),
		utils.NewNullString(p.Category_1), utils.NewNullString(p.Icon),
		utils.NewNullString(p.Episodesystem), utils.NewNullString(p.Episodenum),
		utils.NewNullString(p.Ratingsystem), utils.NewNullString(p.Ratingvalue),
		utils.NewNullString(p.Lang))
	if err != nil {
		// Return only error.
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Warnln("Error retrieving the ID: %v", err)
		return 0, err
	}

	// This query returns nothing.
	return id, nil
}

// Update Epg Programme method for updating a programme by given Channel object.
func (q *EpgQueries) UpdateEpgProgramme(id int64, p *models.EpgProgramme) error {
	// Define query string.
	query := `UPDATE epgprogramme SET start = $2, stop = $3, channel = $4, title = $5, 
	subtitle = $6, desc = $7, credits = $8, date = $9, category_1 = $10, category_2 = $11, 
	category_3 = $12,  category_4 = $13, icon = $14, episodesystem = $15, episodenum = $16, 
	ratinsystem = $17, ratingvalue = $18, lang = $19, WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id, p.Start, p.Stop, utils.NewNullString(p.Channel),
		utils.NewNullString(p.Title), utils.NewNullString(p.Subtitle),
		utils.NewNullString(p.Desc), utils.NewNullString(p.Credits),
		utils.NewNullString(p.Date), utils.NewNullString(p.Category_1),
		utils.NewNullString(p.Category_1), utils.NewNullString(p.Category_1),
		utils.NewNullString(p.Category_1), utils.NewNullString(p.Icon),
		utils.NewNullString(p.Episodesystem), utils.NewNullString(p.Episodenum),
		utils.NewNullString(p.Ratingsystem), utils.NewNullString(p.Ratingvalue),
		utils.NewNullString(p.Lang))
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// Delete Epg Programme method for delete programme by given ID.
func (q *EpgQueries) DeleteEpgProgramme(id int64) error {
	// Define query string.
	query := `DELETE FROM epgprogramme WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}
