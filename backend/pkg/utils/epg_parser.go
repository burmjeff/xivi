package utils

import (
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/rs/zerolog/log"
)

// EpgParser - A parser for epg xml files.
type EpgParser struct {
	Db *database.Queries
}

func (m *EpgParser) ParseEpg(playlistID int64, path string) {
	epg := models.EpgItem{}

	log.Info().Msg("EPG Parser started")

	if isValidURL(path) {
		log.Info().Msg("Started parsing xml URL...")
		resp, err := http.Get(path)
		if err != nil {
			log.Error().Msgf("Unable to get epg.xml FILE: %v", err)
			return
		}
		epg, err = parseXML(resp.Body)
		if err != nil {
			log.Error().Msgf("Unable to parse epg.xml FILE: %v", err)
			return
		}
		defer resp.Body.Close()
	} else {
		log.Info().Msg("Started parsing xml file...")
		// Open the XML file
		file, err := os.Open(path)
		if err != nil {
			log.Error().Msgf("Unable to get epg.xml FILE: %v", err)
			return
		}
		epg, err = parseXML(file)
		if err != nil {
			log.Error().Msgf("Unable to parse epg.xml FILE: %v", err)
			return
		}
		defer file.Close()
	}

	vectorIn := make(chan string)
	//vectorOut := make(chan interface{})
	go VectorQueue(vectorIn, m.Db)

	// Print out the parsed data
	for _, channel := range epg.Channels {
		if channel.ChannelId != "" {
			_, err := m.Db.GetEpgChannelByChannelId(channel.ChannelId)
			if err != nil {
				log.Info().Msgf("EPG XML PARSER: Creating new channel: %s", channel.DisplayName)
				_, err = m.Db.CreateEpgChannel(&channel)
				if err != nil {
					log.Error().Msgf("EPG XML PARSER: Failed to create new EPG channel: %v", err)
					continue
				}
			} else {
				log.Info().Msgf("EPG XML PARSER: Channel already exists: %s", channel.DisplayName)
			}
			vectorIn <- channel.ChannelId
		}

	}

	close(vectorIn)

	for _, programme := range epg.Programmes {
		//Convert times
		if !programme.Start.IsZero() {

			if programme.Channel != "" {
				FoundProg, err := m.Db.GetEpgProgrammeByChannelandTime(programme.Channel, programme.Start)
				if err != nil {
					log.Info().Msgf("EPG XML PARSER: Creating new Programme: %s", programme.Title.Value)
					m.Db.CreateEpgProgramme(&programme)
					if err != nil {
						log.Warn().Msgf("EPG XML PARSER: Failed to create new programme: %v", err)
						continue
					}
				} else {
					log.Info().Msgf("EPG XML PARSER: Updating Programme: %s", programme.Title.Value)
					m.Db.UpdateEpgProgramme(FoundProg.ID, &programme)
					if err != nil {
						log.Warn().Msgf("EPG XML PARSER: Failed to update programme: %v", err)
						continue
					}
				}
			}
		} else {
			log.Error().Msg("EPG XML PARSER: NO PROGRAMMES FOUND")
		}

	}
	log.Info().Msg("EPG Parser Finished")

	// Get all templates.
	templates, err := m.Db.GetTemplates()
	if err != nil {
		log.Error().Msg("EPG XML PARSER: NO TEMPLATE FOUND")
	} else {
		for _, template := range templates {
			go CreateEpgXML(m.Db, template)
		}
	}

}

func parseXML(xmlData io.Reader) (models.EpgItem, error) {
	var epg models.EpgItem
	decoder := xml.NewDecoder(xmlData)

	for {
		// Read tokens from the XML data stream
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return models.EpgItem{}, err
		}

		// If the token is a start element, check the element name and decode the corresponding struct
		if se, ok := token.(xml.StartElement); ok {
			switch se.Name.Local {
			case "channel":
				var channel models.EpgChannel
				if err := decoder.DecodeElement(&channel, &se); err != nil {
					return models.EpgItem{}, err
				}
				epg.Channels = append(epg.Channels, channel)
			case "programme":
				var programme models.EpgProgramme
				if err := decoder.DecodeElement(&programme, &se); err != nil {
					return models.EpgItem{}, err
				}
				epg.Programmes = append(epg.Programmes, programme)
			}
		}
	}

	return epg, nil
}
