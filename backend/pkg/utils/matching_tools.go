package utils

import (
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
)

// Search for filter match
func SearchFilter(db *database.Queries, id string) (string, error) {
	match, err := db.GetFilter(id)
	if err != nil {
		return "", err
	}
	return match.NewName, nil
}

// Search if tvgid matches for playlist channel and add to template if match
func MatchChanneltoTemplate(db *database.Queries, channel *models.PlaylistChannel) error {
	tmplChannelItem := &models.TemplateChannelItem{}
	channelMatch := &models.TemplateChannel{}
	tmplChannelItem.PlaylistChannelId = channel.ID

	filterMatch, err := SearchFilter(db, channel.TvgID)
	if err != nil {
		*channelMatch, err = db.GetTmplChannelBytvgid(channel.TvgID)
		if err != nil {
			return err
		}
	} else {
		*channelMatch, err = db.GetTmplChannelBytvgid(filterMatch)
		if err != nil {
			return err
		}
	}

	tmplChannelItem.ChannelId = channelMatch.ID

	_, err = db.CreateTmplChannelItem(tmplChannelItem)
	if err != nil {
		return err
	}

	return nil
}

// Create filter, dont create if template channel exists with same tvgid
func CreateFilter(db *database.Queries, oldName string, newName string) error {
	channelFilter := &models.ChannelFilter{OldName: oldName, NewName: newName}
	_, err := db.GetTmplChannelBytvgid(oldName)
	if err != nil {
		err := db.CreateFilter(*channelFilter)
		if err != nil {
			return err
		}
	}
	return nil
}
