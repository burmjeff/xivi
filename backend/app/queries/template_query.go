package queries

import (
	"database/sql"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// TemplateQueries struct for queries from Template model.
type TemplateQueries struct {
	*sqlx.DB
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

// GetTemplateGroupChannel by channel_id
func (q *TemplateQueries) GetTmplGroupChannelsByChannel(channel_id int64) (*[]models.TemplateGroupChannel, error) {
	tmplGroupChannels := &[]models.TemplateGroupChannel{}

	query := `SELECT * FROM template_group_channel WHERE channel_id = ?`

	err := q.Select(tmplGroupChannels, query, channel_id)
	if err != nil {
		return nil, err
	}

	// Return query result.
	return tmplGroupChannels, nil
}

func (q *TemplateQueries) GetTmplGroupChannelsByTmpl(template_id int64) (*[]models.TemplateGroupChannel, error) {
	tmplGroupChannels := &[]models.TemplateGroupChannel{}

	query := `SELECT template_group_channel.* FROM template_group_channel 
		JOIN template_group_item ON template_group_channel.group_id = template_group_item.group_id
		WHERE template_group_item.template_id = ? 
		ORDER BY orderr ASC`

	err := q.Select(tmplGroupChannels, query, template_id)
	if err != nil {
		return tmplGroupChannels, err
	}

	return tmplGroupChannels, nil
}

func (q *TemplateQueries) GetTmplChannelsByGroup(groupId int64) ([]models.TemplateChannel, error) {
	channels := []models.TemplateChannel{}

	query := `SELECT templatechannel.* FROM templatechannel 
		JOIN template_group_channel ON templatechannel.id = template_group_channel.channel_id
		WHERE template_group_channel.group_id = ? 
		ORDER BY template_group_channel.orderr ASC`

	err := q.Select(&channels, query, groupId)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Delete a templategroupitem by given ID.
func (q *TemplateQueries) DeleteTmplGroupItem(p *models.TemplateGroupItem) error {
	query := `DELETE FROM template_group_item WHERE template_id = ? AND group_id = ?`

	_, err := q.Exec(query, p.TemplateId, p.GroupId)
	if err != nil {
		return err
	}

	return nil
}

// Get template channels by template ID.
func (q *TemplateQueries) GetTmplChannels(id int64) (*[]models.TemplateChannel, error) {
	channels := &[]models.TemplateChannel{}

	query := `SELECT templatechannel.* FROM templatechannel 
	JOIN template_group_channel ON templatechannel.id = template_group_channel.channel_id 
	JOIN template_group_item ON template_group_item.group_id = template_group_channel.group_id 
	WHERE template_group_item.template_id = ? 
	ORDER BY template_group_channel.orderr ASC`

	err := q.Select(channels, query, id)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Get all channel tvgids by Template ID.
func (q *TemplateQueries) GetTmplTvgids(id int64) (*[]string, error) {
	channels := &[]string{}

	query := `SELECT templatechannel.tvgid FROM templatechannel
		JOIN template_group_channel ON templatechannel.id = template_group_channel.channel_id
		JOIN template_group_item ON template_group_item.group_id = template_group_channel.group_id
		WHERE template_group_item.template_id = ? AND templatechannel.tvgid IS NOT NULL`

	err := q.Select(channels, query, id)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Get a template channel by given ID.
func (q *TemplateQueries) GetTmplChannel(id int64) (*models.TemplateChannel, error) {
	channel := &models.TemplateChannel{}

	query := `SELECT * FROM templatechannel WHERE id = ?`

	err := q.Get(channel, query, id)
	if err != nil {
		return nil, err
	}

	return channel, nil
}

// Get a template channel by name.
func (q *TemplateQueries) GetTmplChannelByName(name string) (*models.TemplateChannel, error) {
	channel := &models.TemplateChannel{}

	query := `SELECT * FROM templatechannel WHERE name = ?`

	err := q.Get(channel, query, name)
	if err != nil {
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
		return nil, err
	}

	return channels, nil
}

// Create a template Channel by given Channel object.
func (q *TemplateQueries) CreateTmplChannel(p *models.TemplateChannel) (int64, error) {
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

// Update a template channel by given channel object.
func (q *TemplateQueries) UpdateTmplChannel(t models.TemplateChannel) error {
	query := `UPDATE templatechannel SET name = ?, tvgid = ?, logoid = ? WHERE id = ?`

	_, err := q.Exec(query, t.Name, t.TvgID, t.LogoId, t.ID)
	if err != nil {
		return err
	}

	return nil
}

// Delete a template channel by ID.
func (q *TemplateQueries) DeleteTmplChannel(id int64) error {
	query := `DELETE FROM templatechannel WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}

// GetTemplateChannelItem
func (q *TemplateQueries) GetTmplChannelItem(item *models.TemplateChannelItem) (*models.TemplateChannelItem, error) {
	templatechannelitem := &models.TemplateChannelItem{}

	query := `SELECT templatechannelitem.* FROM templatechannelitem WHERE channel_id = ? AND playlist_channel_id = ?`

	if err := q.Get(templatechannelitem, query, item.ChannelId, item.PlaylistChannelId); err != nil {
		return nil, err
	}

	return templatechannelitem, nil
}

// GetTemplateChannelItems by channel_id
func (q *TemplateQueries) GetTmplChannelItemsByCh(id int64) (*[]models.TemplateChannelItem, error) {
	templatechannelitems := &[]models.TemplateChannelItem{}

	query := `SELECT * FROM templatechannelitem WHERE channel_id = ?`

	if err := q.Select(templatechannelitems, query, id); err != nil {
		return nil, err
	}

	return templatechannelitems, nil
}

// GetTemplateChannelItems by playlist_channel_id
func (q *TemplateQueries) GetTmplChannelItemsByPl(id int64) (*[]models.TemplateChannelItem, error) {
	templatechannelitems := &[]models.TemplateChannelItem{}

	query := `SELECT * FROM templatechannelitem WHERE playlist_channel_id = ?`

	if err := q.Select(templatechannelitems, query, id); err != nil {
		return nil, err
	}

	return templatechannelitems, nil
}

// GetTemplateChannelItems method for getting all items by ID.
func (q *TemplateQueries) GetTmplChannelItems(id int64) (*[]models.TemplateChannelItem, error) {
	channelitems := &[]models.TemplateChannelItem{}

	query := `SELECT * FROM templatechannelitem WHERE id = ?`

	if err := q.Select(channelitems, query, id); err != nil {
		return nil, err
	}

	return channelitems, nil
}

// Create a TemplateChannelItem by given object.
func (q *TemplateQueries) CreateTmplChannelItem(p *models.TemplateChannelItem) (int64, error) {
	query := `INSERT INTO templatechannelitem VALUES (null, ?, ?, ?)`

	res, err := q.Exec(query, p.ChannelId, p.PlaylistChannelId, p.Order)
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

// Update a TemplateChannelItem by given object.
func (q *TemplateQueries) UpdateTmplChannelItem(id int64, p *models.TemplateChannelItem) error {
	query := `UPDATE templatechannelitem SET channel_id = ?, playlist_channel_id = ?, orderr = ? WHERE id = ?`

	_, err := q.Exec(query, p.ChannelId, p.PlaylistChannelId, p.Order, id)
	if err != nil {
		return err
	}

	return nil
}

// Delete a TemplateChannelItem by given ID.
func (q *TemplateQueries) DeleteTmplChannelItem(templateItem *models.TemplateChannelItem) error {
	query := `DELETE FROM templatechannelitem 
	WHERE channel_id = ?
	AND playlist_channel_id = ?`

	_, err := q.Exec(query, templateItem.ChannelId, templateItem.PlaylistChannelId)
	if err != nil {
		return err
	}

	return nil
}

// Get Templategroupitems by template id
func (q *TemplateQueries) GetTmplGroupItems(id int64) (*[]models.TemplateGroupItem, error) {
	templategroupitems := &[]models.TemplateGroupItem{}

	query := `SELECT * FROM template_group_item WHERE template_id = ?`

	err := q.Select(templategroupitems, query, id)
	if err != nil {
		return nil, err
	}

	return templategroupitems, nil
}

// Get Templategroupitems by group id
func (q *TemplateQueries) GetTmplGroupItemsByGroup(id int64) (*[]models.TemplateGroupItem, error) {
	templategroupitems := &[]models.TemplateGroupItem{}

	query := `SELECT * FROM template_group_item WHERE group_id = ?`

	err := q.Select(&templategroupitems, query, id)
	if err != nil {
		return nil, err
	}

	return templategroupitems, nil
}
