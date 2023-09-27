package queries

import (
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
)

// TemplateQueries struct for queries from Template model.
type StreamQueries struct {
	*sqlx.DB
}

func (q *StreamQueries) GetChannelsbyUuid(uuid string) ([]models.ChannelUrl, error) {
	// Define group variable.
	channels := []models.ChannelUrl{}

	// Define query string.
	query := `SELECT channelurl.* FROM channelurl
		JOIN templatechannelitem ON channel.playlist_channel_id = templatechannelitem.playlist_channel_id
		JOIN templatechannel ON templatechannel.id = templatechannelitem.channel_id
		WHERE templatechannel.uuid = ? 
		ORDER BY channelurl.orderr ASC`

	// Send query to database.
	err := q.Select(&channels, query, uuid)
	if err != nil {
		// Return empty object and error.
		return channels, err
	}

	// Return query result.
	return channels, nil
}
