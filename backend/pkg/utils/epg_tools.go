package utils

import (
	"encoding/xml"
	"fmt"
	"os"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

func CreateEpgXML(template models.Template) {
	log.Info().Msg("Create EPG XML: STARTED")
	epg := models.EpgItem{
		GeneratorInfo:  settings.APP_SETTINGS.Application.AppName,
		SourceInfoName: fmt.Sprintf("%s - %s", settings.APP_SETTINGS.Application.AppName, settings.APP_SETTINGS.Application.AppVersion),
	}

	channels, err := database.Db.GetTmplChannels(template.ID)
	if err != nil {
		log.Error().Msgf("No tvgids found for template: %v", template.ID)
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

	_, err = file.Write([]byte(xml.Header))
	if err != nil {
		fmt.Println("Error writing to XML file:", err)
		return
	}

	for _, channel := range *channels {
		epgChannel, err := database.Db.GetEpgChannelByChannelId(channel.TvgID)
		if err != nil {
			log.Warn().Msgf("No EPG channel found for %s: %v", channel, err)
			epgChannel = &models.EpgChannel{
				ChannelId:   *channel.TvgID,
				DisplayName: channel.Name,
			}
		}
		epgChannel.Icon.Src = fmt.Sprintf("http://%s:%d/%s", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port, GetLogoUrl(channel.Uuid))
		epg.Channels = append(epg.Channels, *epgChannel)
	}

	for _, channel := range epg.Channels {
		epgProgrammes, err := database.Db.GetProgrammesBytvgid(channel.ChannelId)
		if epgProgrammes == nil || len(*epgProgrammes) == 0 {
			log.Warn().Msgf("No EPG programme found for %s: %v", channel.ChannelId, err)

			timeNow := &models.Time{Time: time.Now().Truncate(time.Hour)}
			timeFrame := 2

			for i := 0; i <= 48; i += timeFrame {
				programme := &models.EpgProgramme{
					Start:   &models.Time{Time: timeNow.Add(time.Hour * time.Duration(i))},
					Stop:    &models.Time{Time: timeNow.Add(time.Hour * time.Duration(i+timeFrame))},
					Channel: channel.ChannelId,
					Title: models.Title{
						Value: channel.DisplayName,
					},
					Desc: channel.DisplayName,
				}

				epg.Programmes = append(epg.Programmes, *programme)
			}
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

func RemoveEpg(epg *models.Epg) {
	file := fmt.Sprintf("%s/%s.m3u", settings.EPG_FILEPATH, epg.Name)
	if err := os.Remove(file); err != nil {
		fmt.Println("Error removing file:", err)
		return
	}
}
