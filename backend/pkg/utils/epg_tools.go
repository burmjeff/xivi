package utils

import (
	"encoding/xml"
	"fmt"
	"os"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	log "github.com/sirupsen/logrus"
)

type EpgTools struct {
	Db *database.Queries
}

func (m *EpgTools) CreateEpgXML(template models.Template) {
	log.Info("Create EPG XML: STARTED")
	epg := models.EpgItem{
		GeneratorInfo:  os.Getenv("APP_NAME"),
		SourceInfoName: fmt.Sprintf("%s - %s", os.Getenv("APP_NAME"), os.Getenv("APP_VERSION")),
	}

	channelIDs, err := m.Db.GetTmplTvgids(template.ID)
	if err != nil {
		log.Error("No tvgids found for template: ", template.ID)
		return
	}

	file, err := os.Create(fmt.Sprintf("%s/epg/%s.xml", os.Getenv("STREAM_PATH"), template.Name))
	if err != nil {
		log.Error("Error creating file: %v", err)
		return
	}
	defer file.Close()

	encoder := xml.NewEncoder(file)
	encoder.Indent("", "  ")

	if err := encoder.Encode(xml.Header); err != nil {
		log.Error(err)
		return
	}

	for _, channel := range channelIDs {
		epgChannel, err := m.Db.GetEpgChannelByChannelId(channel)
		if err != nil {
			log.Warn("No channel found for tvgid: ", channel, err)
			continue
		}
		epg.Channels = append(epg.Channels, epgChannel)
	}

	for _, programme := range channelIDs {
		epgProgrammes, err := m.Db.GetProgrammesByChannelId(programme)
		if err != nil {
			log.Warn("No programme found for tvgid: ", programme, err)
			continue
		}
		epg.Programmes = append(epg.Programmes, *epgProgrammes...)
	}

	if err := encoder.Encode(epg); err != nil {
		log.Error(err)
		return
	}
	log.Info("Create EPG XML: FINISHED")

}
