package queries

import (
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
)

type StreamQueries struct {
	*sqlx.DB
}

func (q *StreamQueries) GetChannelsbyUuid(uuid string) (*[]models.ChannelUrl, error) {
	channels := &[]models.ChannelUrl{}

	query := `SELECT channelurl.* FROM channelurl
		JOIN templatechannelitem ON channelurl.channel_id = templatechannelitem.playlist_channel_id
		JOIN templatechannel ON templatechannel.id = templatechannelitem.channel_id
		WHERE templatechannel.uuid = ? 
		ORDER BY channelurl.orderr ASC`

	err := q.Select(channels, query, uuid)
	if err != nil {
		return nil, err
	}

	return channels, nil
}
