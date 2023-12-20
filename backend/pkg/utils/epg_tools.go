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

	for _, channel := range channels {
		epgChannel, err := database.Db.GetEpgChannelByChannelId(channel.TvgID)
		if err != nil {
			log.Warn().Msgf("No channel found for %s: %v", channel, err)
			continue
		}
		epgChannel.Icon.Src = fmt.Sprintf("http://%s:%d/%s", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port, GetLogoUrl(channel.Uuid))
		epg.Channels = append(epg.Channels, epgChannel)
	}

	for _, channel := range channels {
		epgProgrammes, err := database.Db.GetProgrammesByChannelId(channel.TvgID)
		if err != nil {
			log.Warn().Msgf("No programme found for %s: %v", channel.TvgID, err)
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
