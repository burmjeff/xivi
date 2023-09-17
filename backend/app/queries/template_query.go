package queries

import (
	"xivi/backend/app/models"
	utils "xivi/backend/pkg/dbutils"

	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
)

// TemplateQueries struct for queries from Template model.
type TemplateQueries struct {
	*sqlx.DB
}

// GetTemplates method
func (q *TemplateQueries) GetTemplates() ([]models.Template, error) {
	templates := []models.Template{}

	// Define query string.
	query := `SELECT * FROM template`

	// Send query to database.
	err := q.Select(&templates, query)
	if err != nil {
		// Return empty object and error.
		return templates, err
	}

	// Return query result.
	return templates, nil
}

// GetTemplate method for getting one template by given ID.
func (q *TemplateQueries) GetTemplate(id int64) (models.Template, error) {
	template := models.Template{}

	// Define query string.
	query := `SELECT * FROM template WHERE id = $1`

	// Send query to database.
	err := q.Get(&template, query, id)
	if err != nil {
		// Return empty object and error.
		return template, err
	}

	// Return query result.
	return template, nil
}

// CreateTemplate method for creating a template by given Template object.
func (q *TemplateQueries) CreateTemplate(p *models.Template) error {
	// Define query string.
	query := `INSERT INTO template VALUES (null, $1)`

	// Send query to database.
	_, err := q.Exec(query, p.Name)
	if err != nil {
		// Return only error.
		return err
	}

	if err != nil {
		log.Warnln("Error retrieving the ID: %v", err)
	}

	// This query returns nothing.
	return nil
}

// UpdateTemplate method for updating template by given Template object.
func (q *TemplateQueries) UpdateTemplate(id int64, p *models.Template) error {
	// Define query string.
	query := `UPDATE template SET name = $2 WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id, p.Name)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// DeleteTemplate method for delete template by given ID.
func (q *TemplateQueries) DeleteTemplate(id int64) error {
	// Define query string.
	query := `DELETE FROM template WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// CreateTemplate method for creating a template by given Template object.
func (q *TemplateQueries) CreateTmplGroupItem(p *models.TemplateGroupItem) error {
	// Define query string.
	query := `INSERT INTO template_group_item VALUES (?, ?, ?)`

	// Send query to database.
	_, err := q.Exec(query, p.TemplateId, p.GroupId, p.Order)
	if err != nil {
		// Return only error.
		return err
	}

	if err != nil {
		log.Warnln("Error retrieving the ID: %v", err)
		return err
	}

	// This query returns nothing.
	return nil
}

// GetGroups method
func (q *TemplateQueries) GetAllTmplGroups() ([]models.TemplateGroup, error) {
	templategroup := []models.TemplateGroup{}

	// Define query string.
	query := `SELECT * FROM templategroup`

	// Send query to database.
	err := q.Select(&templategroup, query)
	if err != nil {
		// Return empty object and error.
		return templategroup, err
	}

	// Return query result.
	return templategroup, nil
}

// GetGroups method
func (q *TemplateQueries) GetTmplGroups(item *models.TemplateGroupItem) ([]models.TemplateGroup, error) {
	templategroup := []models.TemplateGroup{}

	// Define query string.
	query := `SELECT templategroup.* FROM templategroup
	JOIN template_group_item ON templategroup.id = template_group_item.group_id
	WHERE template_group_item.template_id = ?
	ORDER BY template_group_item.orderr ASC`

	// Send query to database.
	err := q.Select(&templategroup, query, item.TemplateId)
	if err != nil {
		// Return empty object and error.
		return templategroup, err
	}

	// Return query result.
	return templategroup, nil
}

// GetGroup method for getting one group by given ID.
func (q *TemplateQueries) GetTmplGroup(id int64) (models.TemplateGroup, error) {
	// Define group variable.
	group := models.TemplateGroup{}

	// Define query string.
	query := `SELECT * FROM templategroup WHERE id = $1`

	// Send query to database.
	err := q.Get(&group, query, id)
	if err != nil {
		// Return empty object and error.
		return group, err
	}

	// Return query result.
	return group, nil
}

// GetChannel method for getting one group by given Name.
func (q *TemplateQueries) GetTmplGroupByName(name string) (models.TemplateGroup, error) {
	// Define group variable.
	group := models.TemplateGroup{}

	// Define query string.
	query := `SELECT * FROM templategroup WHERE name = $1`

	// Send query to database.
	err := q.Get(&group, query, name)
	if err != nil {
		// Return empty object and error.
		return group, err
	}

	// Return query result.
	return group, nil
}

// CreateGroup method for creating group by given Group object.
func (q *TemplateQueries) CreateTmplGroup(p *models.TemplateGroup) (int64, error) {
	// Define query string.
	query := `INSERT INTO templategroup VALUES (null, $1)`

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

// UpdateGroup method for updating group by given Group object.
func (q *TemplateQueries) UpdateTmplGroup(id int64, p *models.TemplateGroup) error {
	// Define query string.
	query := `UPDATE templategroup SET name = $2 WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id, p.Name)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// DeleteGroup method for delete group by given ID.
func (q *TemplateQueries) DeleteTmplGroup(id int64) error {
	// Define query string.
	query := `DELETE FROM templategroup WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// CreateTemplate method for creating a template by given Template object.
func (q *TemplateQueries) CreateTmplGroupChannel(p *models.TemplateGroupChannel) error {
	// Define query string.
	query := `INSERT INTO template_group_channel VALUES (?, ?, ?)`

	// Send query to database.
	_, err := q.Exec(query, p.GroupId, p.ChannelId, p.Order)
	if err != nil {
		// Return only error.
		return err
	}

	if err != nil {
		log.Warnln("Error retrieving the ID: %v", err)
		return err
	}

	// This query returns nothing.
	return nil
}

// GetChannel method for getting one group by given Name.
func (q *TemplateQueries) GetTmplGroupChannels(template_id int64) ([]models.TemplateGroupChannel, error) {
	// Define group variable.
	tmplGroupChannels := []models.TemplateGroupChannel{}

	// Define query string.
	query := `SELECT template_group_channel.* FROM template_group_channel 
		JOIN template_group_item ON template_group_channel.group_id = template_group_item.group_id
		WHERE template_group_item.template_id = ? 
		ORDER BY orderr ASC`

	// Send query to database.
	err := q.Select(&tmplGroupChannels, query, template_id)
	if err != nil {
		// Return empty object and error.
		return tmplGroupChannels, err
	}

	// Return query result.
	return tmplGroupChannels, nil
}

// GetChannel method for getting one group by given Name.
func (q *TemplateQueries) GetTmplChannelsByGroup(item *models.TemplateGroupChannel) ([]models.TemplateChannel, error) {
	// Define group variable.
	tmplChannels := []models.TemplateChannel{}

	// Define query string.
	query := `SELECT templatechannel.* FROM templatechannel 
		JOIN template_group_channel ON templatechannel.id = template_group_channel.channel_id
		WHERE template_group_channel.group_id = ? 
		ORDER BY template_group_channel.orderr ASC`

	// Send query to database.
	err := q.Select(&tmplChannels, query, item.GroupId)
	if err != nil {
		// Return empty object and error.
		return tmplChannels, err
	}

	// Return query result.
	return tmplChannels, nil
}

// DeleteTemplate method for delete template by given ID.
func (q *TemplateQueries) DeleteTmplGroupItem(p *models.TemplateGroupItem) error {
	// Define query string.
	query := `DELETE FROM template_group_item WHERE template_id = ? AND group_id = ?`

	// Send query to database.
	_, err := q.Exec(query, p.TemplateId, p.GroupId)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// GetTmplChannels method for getting all template channels.
func (q *TemplateQueries) GetTmplChannels() ([]models.TemplateChannel, error) {
	channels := []models.TemplateChannel{}

	// Define query string.
	query := `SELECT * FROM templatechannel`

	// Send query to database.
	err := q.Select(&channels, query)
	if err != nil {
		// Return empty object and error.
		return channels, err
	}

	// Return query result.
	return channels, nil
}

// GetChannels method for getting all channels by Template.
func (q *TemplateQueries) GetChannelsByTmpl(id int64) ([]models.TemplateChannel, error) {
	channels := []models.TemplateChannel{}

	// Define query string.
	query := `SELECT * FROM templatechannel WHERE TemplateID = ?`

	// Send query to database.
	err := q.Get(&channels, query, id)
	if err != nil {
		// Return empty object and error.
		return channels, err
	}

	// Return query result.
	return channels, nil
}

// GetChannels method for getting all channel tvgids by Template.
func (q *TemplateQueries) GetTmplTvgids(id int64) ([]string, error) {
	channels := []string{}

	// Define query string.
	query := `SELECT templatechannel.tvgid FROM templatechannel
		JOIN template_group_channel ON templatechannel.id = template_group_channel.channel_id
		WHERE template_group_channel.template_id = ? AND templatechannel.tvgid IS NOT NULL`

	// Send query to database.
	err := q.Select(&channels, query, id)
	if err != nil {
		// Return empty object and error.
		return channels, err
	}

	// Return query result.
	return channels, nil
}

// GetChannel method for getting one channel by given ID.
func (q *TemplateQueries) GetTmplChannel(id int64) (models.TemplateChannel, error) {
	// Define channel variable.
	channel := models.TemplateChannel{}

	// Define query string.
	query := `SELECT * FROM templatechannel WHERE id = ?`

	// Send query to database.
	err := q.Get(&channel, query, id)
	if err != nil {
		// Return empty object and error.
		return channel, err
	}

	// Return query result.
	return channel, nil
}

// GetChannel method for getting one channel by given name.
func (q *TemplateQueries) GetTmplChannelByName(name string) (models.TemplateChannel, error) {
	// Define channel variable.
	channel := models.TemplateChannel{}

	// Define query string.
	query := `SELECT * FROM templatechannel WHERE name = ?`

	// Send query to database.
	err := q.Get(&channel, query, name)
	if err != nil {
		// Return empty object and error.
		return channel, err
	}

	// Return query result.
	return channel, nil
}

// GetChannel method for getting one channel by given tvgid.
func (q *TemplateQueries) GetTmplChannelBytvgid(tvgid string) (models.TemplateChannel, error) {
	// Define channel variable.
	channel := models.TemplateChannel{}

	// Define query string.
	query := `SELECT * FROM templatechannel WHERE tvgid = ?`

	// Send query to database.
	err := q.Get(&channel, query, tvgid)
	if err != nil {
		// Return empty object and error.
		return channel, err
	}

	// Return query result.
	return channel, nil
}

// CreateChannel method for creating a Channel by given Channel object.
func (q *TemplateQueries) CreateTmplChannel(p *models.TemplateChannel) (int64, error) {
	// Define query string.
	query := `INSERT INTO templatechannel VALUES (null, $1, $2, $3, $4)`

	// Send query to database.
	res, err := q.Exec(query, p.Name, utils.NewNullString(p.TvgID), p.LogoId, p.Uuid)
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

// UpdateChannel method for updating a channel by given Channel object.
func (q *TemplateQueries) UpdateTmplChannel(id int64, p *models.TemplateChannel) error {
	// Define query string.
	query := `UPDATE templatechannel SET name = $2, tvgid = $3, logo_id = $4 WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id, p.Name, p.TvgID, p.LogoId)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// DeleteChannel method for delete channel by given ID.
func (q *TemplateQueries) DeleteTmplChannel(id int64) error {
	// Define query string.
	query := `DELETE FROM templatechannel WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// GetTemplates method
func (q *TemplateQueries) GetTmplChannelItemsByCh(id int64) ([]models.TemplateChannelItem, error) {
	templatechannelitems := []models.TemplateChannelItem{}

	// Define query string.
	query := `SELECT * FROM templatechannelitem WHERE channel_id = $1`

	// Send query to database.
	err := q.Select(&templatechannelitems, query, id)
	if err != nil {
		// Return empty object and error.
		return templatechannelitems, err
	}

	// Return query result.
	return templatechannelitems, nil
}

// GetTemplates method
func (q *TemplateQueries) GetTmplChannelItemsByPl(id int64) ([]models.TemplateChannelItem, error) {
	templatechannelitems := []models.TemplateChannelItem{}

	// Define query string.
	query := `SELECT * FROM templatechannelitem WHERE playlist_channel_id = $1`

	// Send query to database.
	err := q.Select(&templatechannelitems, query, id)
	if err != nil {
		// Return empty object and error.
		return templatechannelitems, err
	}

	// Return query result.
	return templatechannelitems, nil
}

// GetTemplate method for getting one template by given ID.
func (q *TemplateQueries) GetTmplChannelItem(id int64) (models.TemplateChannelItem, error) {
	channelitem := models.TemplateChannelItem{}

	// Define query string.
	query := `SELECT * FROM templatechannelitem WHERE id = $1`

	// Send query to database.
	err := q.Get(&channelitem, query, id)
	if err != nil {
		// Return empty object and error.
		return channelitem, err
	}

	// Return query result.
	return channelitem, nil
}

// CreateTemplate method for creating a template by given Template object.
func (q *TemplateQueries) CreateTmplChannelItem(p *models.TemplateChannelItem) (int64, error) {
	// Define query string.
	query := `INSERT INTO templatechannelitem VALUES (null, $1, $2, $3)`

	// Send query to database.
	res, err := q.Exec(query, p.ChannelId, p.PlaylistChannelId, p.Order)
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

// UpdateTemplate method for updating template by given Template object.
func (q *TemplateQueries) UpdateTmplChannelItem(id int64, p *models.TemplateChannelItem) error {
	// Define query string.
	query := `UPDATE templatechannelitem SET channel_id = $2, playlist_channel_id = $3, orderr = $4 WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id, p.ChannelId, p.PlaylistChannelId, p.Order)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// DeleteTemplate method for delete template by given ID.
func (q *TemplateQueries) DeleteTmplChannelItem(id int64) error {
	// Define query string.
	query := `DELETE FROM templatechannelitem WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}
