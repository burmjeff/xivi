package queries

import (
	"strings"
	"xivi/backend/app/models"
	utils "xivi/backend/pkg/dbutils"

	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
)

// EpgQueries struct for queries from Epg model.
type EpgQueries struct {
	*sqlx.DB
}

type Res struct {
	Data []string
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
	query := `SELECT * FROM epg WHERE id = ?`

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
	query := `INSERT INTO epg VALUES (null, ?, ?, ?)`

	// Send query to database.
	res, err := q.Exec(query, p.Name, p.URL, p.Order)
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
	query := `UPDATE epg SET name = ?, url = ?, orderr = ? WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, id, p.Name, p.URL, p.Order)
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
	query := `DELETE FROM epg WHERE id = ?`

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
	query := `SELECT * FROM epgchannel WHERE id = ?`

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
	query := `SELECT * FROM epgchannel WHERE channelid = ? LIMIT 1`

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
	query := `INSERT INTO epgchannel VALUES (null, ?, ?, ?)`

	// Send query to database.
	res, err := q.Exec(query, p.ChannelId, p.DisplayName, p.Icon.Src)
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
	query := `UPDATE epgchannel SET channelid = ?, displayname = ?, icon = ? WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, id, p.ChannelId, p.DisplayName, p.Icon.Src)
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
	query := `DELETE FROM epgchannel WHERE id = ?`

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
func (q *EpgQueries) GetProgrammesByChannelId(channelID string) (*[]models.EpgProgramme, error) {
	programmes := []models.EpgProgramme{}

	// Define query string.
	query := `SELECT * FROM epgprogramme WHERE channel = ?`

	// Send query to database.
	err := q.Select(&programmes, query, channelID)
	if err != nil {
		// Return empty object and error.
		return nil, err
	}

	// Return query result.
	return &programmes, nil
}

// Get Epg Programme method for getting one programme by given ID.
func (q *EpgQueries) GetEpgProgramme(id int64) (*models.EpgProgramme, error) {
	// Define programme variable.
	programme := models.EpgProgramme{}

	// Define query string.
	query := `SELECT * FROM epgprogramme WHERE id = ?`

	// Send query to database.
	err := q.Select(&programme, query, id)
	if err != nil {
		// Return empty object and error.
		return nil, err
	}

	// Return query result.
	return &programme, nil
}

// Get Epg Programme method for getting one programme by given ID.
func (q *EpgQueries) GetEpgProgrammeByChannelandTime(channelID string, start *models.Time) (models.EpgProgramme, error) {

	// Define programme variable.
	programme := models.EpgProgramme{}

	// Define query string.
	query := `SELECT * FROM epgprogramme WHERE channel = ? and start = ? LIMIT 1`

	// Send query to database.
	err := q.Get(&programme, query, channelID, start)
	if err != nil {
		// Return empty object and error.
		return programme, err
	}

	// Return query result.
	return programme, nil
}

// Create Epg Programme method for creating a programme by given object.
func (q *EpgQueries) CreateEpgProgramme(p *models.EpgProgramme) (int64, error) {

	q.MapperFunc(utils.CustomMapper)
	// Define query string.
	query := `INSERT INTO epgprogramme VALUES (null, ?, ?, ?, ?, ?, ?, ?, ?, 
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// Send query to database.
	res, err := q.Exec(query,
		p.Start,
		p.Stop,
		p.Channel,
		p.Title.Value,
		p.Title.Lang,
		p.Subtitle,
		p.Desc,
		strings.Join(p.Categories, ","),
		p.Icon.Src,
		strings.Join(p.Directors, ","),
		strings.Join(p.Presenters, ","),
		strings.Join(p.Producers, ","),
		strings.Join(p.Actors, ","),
		p.EpisodeNumber.System,
		p.EpisodeNumber.Value,
		p.Rating.System,
		p.Rating.Value,
		p.Video.Quality,
		p.Date)
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

	// Build the update query
	query := `UPDATE epgprogramme SET `

	if p.Title.Value != "" {
		query += `title = COALESCE(title, ?), `
	}

	if p.Subtitle != "" {
		query += `subtitle = COALESCE(subtitle, ?), `
	}

	if p.Desc != "" {
		query += `desc = COALESCE(desc, ?), `
	}

	if len(p.Directors) > 0 {
		query += `directors = COALESCE(directors, ?), `
	}

	if len(p.Presenters) > 0 {
		query += `presenters = COALESCE(presenters, ?), `
	}

	if len(p.Producers) > 0 {
		query += `producers = COALESCE(producers, ?), `
	}

	if len(p.Actors) > 0 {
		query += `actors = COALESCE(actors, ?), `
	}

	if p.Date != "" {
		query += `date = COALESCE(date, ?), `
	}

	if len(p.Categories) > 0 {
		query += `categories = COALESCE(categories, ?), `
	}

	if p.Icon.Src != "" {
		query += `icon = COALESCE(icon, ?), `
	}

	if p.EpisodeNumber.System != "" {
		query += `episodesystem = COALESCE(episodesystem, ?), `
	}

	if p.EpisodeNumber.Value != "" {
		query += `episodenum = COALESCE(episodenum, ?), `
	}

	if p.Rating.System != "" {
		query += `ratingsystem = COALESCE(ratingsystem, ?), `
	}

	if p.Rating.Value != "" {
		query += `ratingvalue = COALESCE(ratingvalue, ?), `
	}

	if p.Video.Quality != "" {
		query += `video.quality = COALESCE(video.quality, ?), `
	}

	if p.Title.Lang != "" {
		query += `lang = COALESCE(lang, ?), `
	}

	// Trim the trailing comma and space from the query
	query = strings.TrimSuffix(query, ", ")

	// Add the WHERE clause to match the ID
	query += " WHERE id = ?"

	// Send query to database.
	_, err := q.Exec(query, id,
		p.Title.Value,
		p.Title.Lang,
		p.Subtitle,
		p.Desc,
		p.Icon.Src,
		strings.Join(p.Categories, ","),
		strings.Join(p.Directors, ","),
		strings.Join(p.Presenters, ","),
		strings.Join(p.Producers, ","),
		strings.Join(p.Actors, ","),
		p.EpisodeNumber.System,
		p.EpisodeNumber.Value,
		p.Rating.System,
		p.Rating.Value,
		p.Video.Quality,
		p.Date)
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
	query := `DELETE FROM epgprogramme WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// GetEpgChannelItems method
func (q *EpgQueries) GetEpgChannelItemsByCh(id int64) ([]models.EpgChannelItem, error) {
	epgchannelitems := []models.EpgChannelItem{}

	// Define query string.
	query := `SELECT * FROM epgchannelitem WHERE channel_id = ?`

	// Send query to database.
	err := q.Select(&epgchannelitems, query, id)
	if err != nil {
		// Return empty object and error.
		return epgchannelitems, err
	}

	// Return query result.
	return epgchannelitems, nil
}

// GetEpgChannelItems method
func (q *EpgQueries) GetEpgChannelItemsByProgramme(id int64) ([]models.EpgChannelItem, error) {
	epgchannelitems := []models.EpgChannelItem{}

	// Define query string.
	query := `SELECT * FROM epgchannelitem WHERE epg_programme_id = ?`

	// Send query to database.
	err := q.Select(&epgchannelitems, query, id)
	if err != nil {
		// Return empty object and error.
		return epgchannelitems, err
	}

	// Return query result.
	return epgchannelitems, nil
}

// GetEpgChannelItem method for getting one EpgChannelItem by given ID.
func (q *EpgQueries) GetEpgChannelItem(id int64) (models.EpgChannelItem, error) {
	channelitem := models.EpgChannelItem{}

	// Define query string.
	query := `SELECT * FROM epgchannelitem WHERE id = ?`

	// Send query to database.
	err := q.Get(&channelitem, query, id)
	if err != nil {
		// Return empty object and error.
		return channelitem, err
	}

	// Return query result.
	return channelitem, nil
}

// CreateEpgChannelItem method for creating a template by given EpgChannelItem object.
func (q *EpgQueries) CreateEpgChannelItem(p *models.EpgChannelItem) (int64, error) {
	// Define query string.
	query := `INSERT INTO epgchannelitem VALUES (null, ?, ?, ?)`

	// Send query to database.
	res, err := q.Exec(query, p.EpgChannelId, p.EpgProgrammeId)
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

// UpdateEpgChannelItem method for updating template by given EpgChannelItem object.
func (q *EpgQueries) UpdateEpgChannelItem(id int64, p *models.EpgChannelItem) error {
	// Define query string.
	query := `UPDATE epgchannelitem SET epg_channel_id = ?, epg_programme_id = ? WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, id, p.EpgChannelId, p.EpgProgrammeId)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// DeleteEpgChannelItem method for delete template by given ID.
func (q *EpgQueries) DeleteEpgChannelItem(id int64) error {
	// Define query string.
	query := `DELETE FROM epgchannelitem WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}
