package utils

import (
	"encoding/xml"
	"fmt"
	"os"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

type EpgTools struct {
	Db *database.Queries
}

func (m *EpgTools) CreateEpgXML(template models.Template) {
	log.Info().Msg("Create EPG XML: STARTED")
	epg := models.EpgItem{
		GeneratorInfo:  settings.APP_SETTINGS.Application.AppName,
		SourceInfoName: fmt.Sprintf("%s - %s", settings.APP_SETTINGS.Application.AppName, settings.APP_SETTINGS.Application.AppVersion),
	}

	channelIDs, err := m.Db.GetTmplTvgids(template.ID)
	if err != nil {
		log.Error().Msgf("No tvgids found for template: %s", template.ID)
		return
	}

	file, err := os.Create(fmt.Sprintf("%s/%s.xml", settings.EPG_FILEPATH, template.Name))
	if err != nil {
		log.Error().Msgf("Error creating file: %v", err)
		return
	}
	defer file.Close()

	encoder := xml.NewEncoder(file)
	encoder.Indent("", "  ")

	if err := encoder.Encode(xml.Header); err != nil {
		log.Err(err)
		return
	}

	for _, channel := range channelIDs {
		epgChannel, err := m.Db.GetEpgChannelByChannelId(channel)
		if err != nil {
			log.Warn().Msgf("No channel found for %s: %v", channel, err)
			continue
		}
		epg.Channels = append(epg.Channels, epgChannel)
	}

	for _, programme := range channelIDs {
		epgProgrammes, err := m.Db.GetProgrammesByChannelId(programme)
		if err != nil {
			log.Warn().Msgf("No programme found for %s: %v", programme, err)
			continue
		}
		epg.Programmes = append(epg.Programmes, *epgProgrammes...)
	}

	if err := encoder.Encode(epg); err != nil {
		log.Err(err)
		return
	}
	log.Info().Msg("Create EPG XML: FINISHED")

}
