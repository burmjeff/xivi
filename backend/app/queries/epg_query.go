package queries

import (
	"database/sql"
	"strings"
	"time"
	"xivi/backend/app/models"
	utils "xivi/backend/pkg/dbutils"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// EpgQueries struct for queries from Epg model.
type EpgQueries struct {
	*sqlx.DB
}

type Res struct {
	Data []string
}

// Get Epgs
func (q *EpgQueries) GetEpgs() (*[]models.Epg, error) {
	epgs := &[]models.Epg{}

	query := `SELECT * FROM epg`

	err := q.Select(epgs, query)
	if err != nil {
		return nil, err
	}

	return epgs, nil
}

// Get a epg by given ID.
func (q *EpgQueries) GetEpg(id int64) (*models.Epg, error) {
	epg := &models.Epg{}

	query := `SELECT * FROM epg WHERE id = ?`

	err := q.Get(epg, query, id)
	if err != nil {
		return nil, err
	}

	return epg, nil
}

// Create a epg by given Epg object.
func (q *EpgQueries) CreateEpg(p *models.Epg) (int64, error) {
	query := `INSERT INTO epg VALUES (null, ?, ?, ?, ?, ?)`

	res, err := q.Exec(query, p.Name, p.URL, p.Order, p.CreatedAt, p.UpdatedAt)
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

// Update a epg by given Epg object.
func (q *EpgQueries) UpdateEpg(id int64, p *models.Epg) error {
	query := `UPDATE epg SET name = ?, url = ?, orderr = ?, updated_at = ? WHERE id = ?`

	_, err := q.Exec(query, p.Name, p.URL, p.Order, p.UpdatedAt, id)
	if err != nil {
		return err
	}

	return nil
}

// Delete a epg by given ID.
func (q *EpgQueries) DeleteEpg(id int64) error {
	query := `DELETE FROM epg WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}

// Get Epg Channels
func (q *EpgQueries) GetEpgChannels() (*[]models.EpgChannel, error) {
	epgchannel := &[]models.EpgChannel{}

	query := `SELECT * FROM epgchannel`

	err := q.Select(epgchannel, query)
	if err != nil {
		return nil, err
	}

	return epgchannel, nil
}

// Get a Epg Channel by given ID.
func (q *EpgQueries) GetEpgChannel(id int64) (*models.EpgChannel, error) {
	epgchannel := &models.EpgChannel{}

	query := `SELECT * FROM epgchannel WHERE id = ?`

	err := q.Get(epgchannel, query, id)
	if err != nil {
		return nil, err
	}

	// Return query result.
	return epgchannel, nil
}

// Get a epgchannel by channel id.
func (q *EpgQueries) GetEpgChannelByChannelId(channelID *string) (*models.EpgChannel, error) {
	epgchannel := &models.EpgChannel{}

	query := `SELECT * FROM epgchannel WHERE channelid = ? LIMIT 1`

	err := q.Get(epgchannel, query, channelID)
	if err != nil {
		return nil, err
	}

	return epgchannel, nil
}

// Create a Epg Channel by given Epg Channel object.
func (q *EpgQueries) CreateEpgChannel(p models.EpgChannel) (int64, error) {
	query := `INSERT INTO epgchannel VALUES (null, ?, ?, ?)`

	res, err := q.Exec(query, p.ChannelId, p.DisplayName, p.Icon.Src)
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

// Update a Epg Channel by given Epg Channel object.
func (q *EpgQueries) UpdateEpgChannel(id int64, p *models.EpgChannel) error {
	query := `UPDATE epgchannel SET channelid = ?, displayname = ?, icon = ? WHERE id = ?`

	_, err := q.Exec(query, id, p.ChannelId, p.DisplayName, p.Icon.Src)
	if err != nil {
		return err
	}

	return nil
}

// Delete a Epg Channel by given ID.
func (q *EpgQueries) DeleteEpgChannel(id int64) error {
	query := `DELETE FROM epgchannel WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}

// Get all Epg Programmes.
func (q *EpgQueries) GetEpgProgrammes() (*[]models.EpgProgramme, error) {
	programmes := &[]models.EpgProgramme{}

	query := `SELECT * FROM epgprogramme`

	err := q.Select(programmes, query)
	if err != nil {
		return nil, err
	}

	return programmes, nil
}

// Get Epg Programmes by channel.
func (q *EpgQueries) GetProgrammesBytvgid(tvgid string) (*[]models.EpgProgramme, error) {
	programmes := &[]models.EpgProgramme{}

	query := `SELECT * FROM epgprogramme WHERE channel = ?`

	err := q.Select(programmes, query, tvgid)
	if err != nil {
		return nil, err
	}

	return programmes, nil
}

// Get the current live Epg Programme by given ID.
func (q *EpgQueries) GetProgrammeByTime(tvgid string, epgTime time.Time) (*models.EpgProgramme, error) {
	programme := &models.EpgProgramme{}

	query := `SELECT * FROM epgprogramme WHERE channel = ? AND start <= ? AND stop > ? LIMIT 1`

	err := q.Get(programme, query, tvgid, epgTime, epgTime)
	if err != nil {
		return nil, err
	}

	return programme, nil
}

// Create Epg Programme by given object.
func (q *EpgQueries) CreateEpgProgramme(p models.EpgProgramme) (int64, error) {

	q.MapperFunc(utils.CustomMapper)
	query := `INSERT INTO epgprogramme VALUES (null, ?, ?, ?, ?, ?, ?, ?, ?, 
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

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

// Update a Epg Programme by given Channel object.
func (q *EpgQueries) UpdateEpgProgramme(id int64, p *models.EpgProgramme) error {

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
		return err
	}

	return nil
}

// Delete a Epg Programme by given ID.
func (q *EpgQueries) DeleteEpgProgramme(id int64) error {
	query := `DELETE FROM epgprogramme WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}

// GetEpgChannelItems method
func (q *EpgQueries) GetEpgChannelItemsByCh(id int64) (*[]models.EpgChannelItem, error) {
	epgchannelitems := &[]models.EpgChannelItem{}

	query := `SELECT * FROM epgchannelitem WHERE channel_id = ?`

	err := q.Select(epgchannelitems, query, id)
	if err != nil {
		return nil, err
	}

	return epgchannelitems, nil
}

// GetEpgChannelItems by programme id
func (q *EpgQueries) GetEpgChannelItemsByProgramme(id int64) (*[]models.EpgChannelItem, error) {
	epgchannelitems := &[]models.EpgChannelItem{}

	query := `SELECT * FROM epgchannelitem WHERE epg_programme_id = ?`

	err := q.Select(epgchannelitems, query, id)
	if err != nil {
		return nil, err
	}

	return epgchannelitems, nil
}

// Get a EpgChannelItem by given ID.
func (q *EpgQueries) GetEpgChannelItem(id int64) (*models.EpgChannelItem, error) {
	channelitem := &models.EpgChannelItem{}

	query := `SELECT * FROM epgchannelitem WHERE id = ?`

	err := q.Get(channelitem, query, id)
	if err != nil {
		return nil, err
	}

	// Return query result.
	return channelitem, nil
}

// Create EpgChannelItem method by given EpgChannelItem object.
func (q *EpgQueries) CreateEpgChannelItem(p *models.EpgChannelItem) (int64, error) {
	query := `INSERT INTO epgchannelitem VALUES (null, ?, ?, ?)`

	res, err := q.Exec(query, p.EpgChannelId, p.EpgProgrammeId)
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

// Update EpgChannelItem by given EpgChannelItem object.
func (q *EpgQueries) UpdateEpgChannelItem(id int64, p *models.EpgChannelItem) error {
	query := `UPDATE epgchannelitem SET epg_channel_id = ?, epg_programme_id = ? WHERE id = ?`

	_, err := q.Exec(query, id, p.EpgChannelId, p.EpgProgrammeId)
	if err != nil {
		return err
	}

	return nil
}

// Delete EpgChannelItem by given ID.
func (q *EpgQueries) DeleteEpgChannelItem(id int64) error {
	query := `DELETE FROM epgchannelitem WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}

// Get epgchannel tvgids
func (q *EpgQueries) GetEpgTvgids() ([]string, error) {
	var items []string

	query := `SELECT channelid FROM epgchannel WHERE channelid is NOT NULL`

	err := q.Select(&items, query)
	if err != nil {
		return nil, err
	}

	return items, nil
}
