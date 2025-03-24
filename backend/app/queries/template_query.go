package queries

import (
	"database/sql"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// TemplateQueries struct for queries from Template model.
type TemplateQueries struct {
	BaseQueries
}

// NewTemplateQueries creates a new TemplateQueries instance
func NewTemplateQueries(db *sqlx.DB) *TemplateQueries {
	return &TemplateQueries{
		BaseQueries: NewBaseQueries(db),
	}
}

// GetTemplates
func (q *TemplateQueries) GetTemplates() (*[]models.Template, error) {
	templates := &[]models.Template{}

	// Define query string.
	query := `SELECT * FROM template`

	// Send query to database.
	err := q.Select(templates, query)
	if err != nil {
		// Return empty object and error.
		return nil, err
	}

	// Return query result.
	return templates, nil
}

// GetTemplates without context (for backward compatibility)
func (q *TemplateQueries) GetTemplatesNoCtx() (*[]models.Template, error) {
	return q.GetTemplates()
}

// Get one template by given ID.
func (q *TemplateQueries) GetTemplate(id int64) (*models.Template, error) {
	template := &models.Template{}

	// Define query string.
	query := `SELECT * FROM template WHERE id = ?`

	// Send query to database.
	err := q.Get(template, query, id)
	if err != nil {
		// Return empty object and error.
		return nil, err
	}

	// Return query result.
	return template, nil
}

// Create a template by given Template object.
func (q *TemplateQueries) CreateTemplate(p *models.Template) (int64, error) {
	// Define query string.
	query := `INSERT INTO template VALUES (null, ?)`

	// Send query to database.
	res, err := q.Exec(query, p.Name)
	if err != sql.ErrNoRows && err != nil {
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

// Update a template by given Template object.
func (q *TemplateQueries) UpdateTemplate(t *models.Template) error {
	// Define query string.
	query := `UPDATE template SET name = ? WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, t.Name, t.ID)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// Delete a template by given ID.
func (q *TemplateQueries) DeleteTemplate(id int64) error {
	query := `DELETE FROM template WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}

// CreateTemplate method for creating a template by given Template object.
func (q *TemplateQueries) CreateTmplGroupItem(p *models.TemplateGroupItem) error {
	query := `INSERT INTO template_group_item VALUES (?, ?, ?)`

	_, err := q.Exec(query, p.TemplateId, p.GroupId, p.Order)
	if err != sql.ErrNoRows && err != nil {
		return err
	}

	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return err
	}

	return nil
}

// Get Template Groups
func (q *TemplateQueries) GetAllTmplGroups() (*[]models.TemplateGroup, error) {
	templategroups := &[]models.TemplateGroup{}

	query := `SELECT * FROM templategroup`

	err := q.Select(templategroups, query)
	if err != nil {
		return nil, err
	}

	return templategroups, nil
}

// Get Template Groups by template id
func (q *TemplateQueries) GetTmplGroups(templateId int64) ([]models.TemplateGroup, error) {
	templategroups := []models.TemplateGroup{}

	query := `SELECT templategroup.* FROM templategroup
	JOIN template_group_item ON templategroup.id = template_group_item.group_id
	WHERE template_group_item.template_id = ?
	ORDER BY template_group_item.orderr ASC`

	err := q.Select(&templategroups, query, templateId)
	if err != nil {
		return nil, err
	}

	return templategroups, nil
}

// Get one template group by given ID.
func (q *TemplateQueries) GetTmplGroup(id int64) (*models.TemplateGroup, error) {
	group := &models.TemplateGroup{}

	query := `SELECT * FROM templategroup WHERE id = ?`

	err := q.Get(group, query, id)
	if err != nil {
		return nil, err
	}

	return group, nil
}

// Get a template group by given Name.
func (q *TemplateQueries) GetTmplGroupByName(name string) (*models.TemplateGroup, error) {
	group := &models.TemplateGroup{}

	query := `SELECT * FROM templategroup WHERE name = ?`

	err := q.Get(group, query, name)
	if err != nil {
		return nil, err
	}

	return group, nil
}

// Create a template group by given Group object.
func (q *TemplateQueries) CreateTmplGroup(t *models.TemplateGroup) (int64, error) {

	if t.DynamicGroup != nil && *t.DynamicGroup == 0 {
		t.DynamicGroup = nil
	}

	query := `INSERT INTO templategroup VALUES (null, ?, ?, ?)`

	res, err := q.Exec(query, t.Name, t.Dynamic, t.DynamicGroup)
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

// Update a template group by given Group object.
func (q *TemplateQueries) UpdateTmplGroup(t *models.TemplateGroup) error {

	if t.DynamicGroup != nil && *t.DynamicGroup == 0 {
		t.DynamicGroup = nil
	}

	query := `UPDATE templategroup SET name = ?, dynamic = ?, dynamicgroup = ? WHERE id = ?`

	_, err := q.Exec(query, t.Name, t.Dynamic, t.DynamicGroup, t.ID)
	if err != nil {
		return err
	}

	return nil
}

// Delete a template group by given ID.
func (q *TemplateQueries) DeleteTmplGroup(id int64) error {
	query := `DELETE FROM templategroup WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}

// Create a templategroupchannel by given object.
func (q *TemplateQueries) CreateTmplGroupChannel(p models.TemplateGroupChannel) error {
	query := `INSERT INTO template_group_channel VALUES (?, ?, ?)`

	_, err := q.Exec(query, p.GroupId, p.ChannelId, p.Order)
	if err != sql.ErrNoRows && err != nil {
		return err
	}

	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return err
	}

	return nil
}

// Get Template Group Channels by Channel ID
func (q *TemplateQueries) GetTmplGroupChannelsByChannel(channel_id int64) (*[]models.TemplateGroupChannel, error) {
	groupchannels := &[]models.TemplateGroupChannel{}

	query := `SELECT template_group_channel.* FROM template_group_channel WHERE channel_id = ?`

	err := q.Select(groupchannels, query, channel_id)
	if err != nil {
		return nil, err
	}

	return groupchannels, nil
}

// Get Template Group Channels by Template ID
func (q *TemplateQueries) GetTmplGroupChannelsByTmpl(template_id int64) (*[]models.TemplateGroupChannel, error) {
	groupchannels := &[]models.TemplateGroupChannel{}

	query := `SELECT tgc.* FROM template_group_channel tgc 
	JOIN template_group_item tgi ON tgc.group_id = tgi.group_id 
	WHERE tgi.template_id = ?`

	err := q.Select(groupchannels, query, template_id)
	if err != nil {
		return nil, err
	}

	return groupchannels, nil
}

// Get Template Channels by Group ID
func (q *TemplateQueries) GetTmplChannelsByGroup(groupId int64) ([]models.TemplateChannel, error) {
	channels := []models.TemplateChannel{}

	query := `SELECT tc.* FROM templatechannel tc
	JOIN template_group_channel tgc ON tc.id = tgc.channel_id
	WHERE tgc.group_id = ?
	ORDER BY tgc.orderr ASC`

	err := q.Select(&channels, query, groupId)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Delete a templategroupitem by given object.
func (q *TemplateQueries) DeleteTmplGroupItem(p *models.TemplateGroupItem) error {
	query := `DELETE FROM template_group_item WHERE template_id = ? AND group_id = ?`

	_, err := q.Exec(query, p.TemplateId, p.GroupId)
	if err != nil {
		return err
	}

	return nil
}

// Get Template Channels by Template ID
func (q *TemplateQueries) GetTmplChannels(id int64) ([]models.TemplateChannel, error) {
	channels := []models.TemplateChannel{}

	query := `SELECT tc.* FROM templatechannel tc
	JOIN template_group_channel tgc ON tc.id = tgc.channel_id
	JOIN template_group_item tgi ON tgc.group_id = tgi.group_id
	WHERE tgi.template_id = ?
	ORDER BY tgc.orderr ASC`

	err := q.Select(&channels, query, id)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Get TvgIDs from Template channels
func (q *TemplateQueries) GetTmplTvgids(id int64) (*[]string, error) {
	channels := &[]string{}

	query := `SELECT tvgid FROM templatechannel tc
	JOIN template_group_channel tgc ON tc.id = tgc.channel_id
	JOIN template_group_item tgi ON tgc.group_id = tgi.group_id
	WHERE tgi.template_id = ?
	AND tvgid IS NOT NULL`

	err := q.Select(channels, query, id)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Get one template channel by given ID.
func (q *TemplateQueries) GetTmplChannel(id int64) (*models.TemplateChannel, error) {
	channel := &models.TemplateChannel{}

	query := `SELECT * FROM templatechannel WHERE id = ?`

	err := q.Get(channel, query, id)
	if err != nil {
		return nil, err
	}

	return channel, nil
}

// Get one template channel by given name.
func (q *TemplateQueries) GetTmplChannelByName(name string) (*models.TemplateChannel, error) {
	channel := &models.TemplateChannel{}

	query := `SELECT * FROM templatechannel WHERE name = ?`

	err := q.Get(channel, query, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return channel, nil
}

// Get template channels by tvgid.
func (q *TemplateQueries) GetTmplChannelsBytvgid(tvgid *string) (*[]models.TemplateChannel, error) {
	channels := &[]models.TemplateChannel{}

	query := `SELECT * FROM templatechannel WHERE tvgid = ?`

	err := q.Select(channels, query, tvgid)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return channels, nil
}

// Create a template channel by given Channel object.
func (q *TemplateQueries) CreateTmplChannel(p models.TemplateChannel) (int64, error) {
	query := `INSERT INTO templatechannel VALUES (null, ?, ?, ?, ?)`

	res, err := q.Exec(query, p.Name, p.TvgID, p.LogoId, p.Uuid)
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

// Update a template channel by given Channel object.
func (q *TemplateQueries) UpdateTmplChannel(t models.TemplateChannel) error {
	query := `UPDATE templatechannel SET name = ?, tvgid = ?, logoid = ? WHERE id = ?`

	_, err := q.Exec(query, t.Name, t.TvgID, t.LogoId, t.ID)
	if err != nil {
		return err
	}

	return nil
}

// Delete a template channel by given ID.
func (q *TemplateQueries) DeleteTmplChannel(id int64) error {
	query := `DELETE FROM templatechannel WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}

// Get one template channel item by given ID.
func (q *TemplateQueries) GetTmplChannelItem(item *models.TemplateChannelItem) (*models.TemplateChannelItem, error) {
	channel := &models.TemplateChannelItem{}

	query := `SELECT * FROM templatechannelitem WHERE channel_id = ? AND playlist_channel_id = ?`

	err := q.Get(channel, query, item.ChannelId, item.PlaylistChannelId)
	if err != nil {
		return nil, err
	}

	return channel, nil
}

// Get Template Channel Items by Channel ID
func (q *TemplateQueries) GetTmplChannelItemsByCh(id int64) (*[]models.TemplateChannelItem, error) {
	channelitems := &[]models.TemplateChannelItem{}

	query := `SELECT * FROM templatechannelitem WHERE channel_id = ?`

	err := q.Select(channelitems, query, id)
	if err != nil {
		return nil, err
	}

	return channelitems, nil
}

// Get Template Channel Items by Playlist Channel ID
func (q *TemplateQueries) GetTmplChannelItemsByPl(id int64) (*[]models.TemplateChannelItem, error) {
	channelitems := &[]models.TemplateChannelItem{}

	query := `SELECT * FROM templatechannelitem WHERE playlist_channel_id = ?`

	err := q.Select(channelitems, query, id)
	if err != nil {
		return nil, err
	}

	return channelitems, nil
}

// Get Template Channel Items
func (q *TemplateQueries) GetTmplChannelItems(id int64) (*[]models.TemplateChannelItem, error) {
	channelitems := &[]models.TemplateChannelItem{}

	query := `SELECT tci.* FROM templatechannelitem tci 
	JOIN template_group_channel tgc ON tci.channel_id = tgc.channel_id 
	JOIN template_group_item tgi ON tgc.group_id = tgi.group_id 
	WHERE tgi.template_id = ?`

	err := q.Select(channelitems, query, id)
	if err != nil {
		return nil, err
	}

	return channelitems, nil
}

// Check if channel item exists from templateid, channelid and playlistid
func (q *TemplateQueries) TmplChannelExists(channelId int64, playlistId int64) (bool, error) {
	var exists bool
	query := `SELECT COUNT(*) > 0 FROM templatechannelitem tci
	JOIN playlistchannel pc ON tci.playlist_channel_id = pc.id
	JOIN template_group_channel tgc ON tci.channel_id = tgc.channel_id
	JOIN templategroup tg ON tgc.group_id = tg.id
	JOIN template_group_item tgi ON tg.id = tgi.group_id
	JOIN template tm ON tgi.template_id = tm.id
	WHERE tci.channel_id = ?
	AND pc.playlist_id = ?`

	err := q.Get(&exists, query, channelId, playlistId)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// Create a template channel item by given Channel Item object.
func (q *TemplateQueries) CreateTmplChannelItem(p *models.TemplateChannelItem) (int64, error) {
	query := `INSERT INTO templatechannelitem VALUES (null, ?, ?, ?)`

	res, err := q.Exec(query, p.ChannelId, p.PlaylistChannelId, p.Order)
	if err != sql.ErrNoRows && err != nil {
		log.Warn().Err(err).Msg("CREATE TMPL CHANNEL ITEM")
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return 0, err
	}

	return id, nil
}

// Update a template channel item by given Channel Item object.
func (q *TemplateQueries) UpdateTmplChannelItem(id int64, p *models.TemplateChannelItem) error {
	query := `UPDATE templatechannelitem SET channel_id = ?, playlist_channel_id = ? WHERE id = ?`

	_, err := q.Exec(query, p.ChannelId, p.PlaylistChannelId, id)
	if err != nil {
		return err
	}

	return nil
}

// Delete a template channel item by given object.
func (q *TemplateQueries) DeleteTmplChannelItem(templateItem *models.TemplateChannelItem) error {
	query := `DELETE FROM templatechannelitem 
	WHERE channel_id = ? AND playlist_channel_id = ?`

	_, err := q.Exec(query, templateItem.ChannelId, templateItem.PlaylistChannelId)
	if err != nil {
		return err
	}

	return nil
}

// Get Template Group Items by Template ID
func (q *TemplateQueries) GetTmplGroupItems(id int64) (*[]models.TemplateGroupItem, error) {
	templateGroupItems := &[]models.TemplateGroupItem{}

	query := `SELECT * FROM template_group_item WHERE template_id = ?`

	err := q.Select(templateGroupItems, query, id)
	if err != nil {
		return nil, err
	}

	return templateGroupItems, nil
}

// Get Template Group Items by Group ID
func (q *TemplateQueries) GetTmplGroupItemsByGroup(id int64) (*[]models.TemplateGroupItem, error) {
	templateGroupItems := &[]models.TemplateGroupItem{}

	query := `SELECT * FROM template_group_item WHERE group_id = ?`

	err := q.Select(templateGroupItems, query, id)
	if err != nil {
		return nil, err
	}

	return templateGroupItems, nil
}
