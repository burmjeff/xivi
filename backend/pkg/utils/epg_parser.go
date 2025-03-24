package utils

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/rs/zerolog/log"
)

func ParseEpg(epg *models.Epg) {
	epgItem := models.EpgItem{}
	ctx := context.Background()

	log.Info().Msg("EPG Parser started")

	if isValidURL(epg.URL) {
		log.Info().Msg("Started parsing xml URL...")
		resp, err := http.Get(epg.URL)
		if err != nil {
			log.Error().Msgf("Unable to get epg.xml FILE: %v", err)
			return
		}
		defer resp.Body.Close()

		bufReader := bufio.NewReader(resp.Body)
		testBytes, err := bufReader.Peek(2)
		if err != nil {
			log.Error().Msgf("Unable to peek at epg.xml FILE: %v", err)
			return
		}
		if testBytes[0] == 31 && testBytes[1] == 139 { //Check if gzip
			gzipReader, err := gzip.NewReader(bufReader)
			if err != nil {
				log.Error().Msgf("Unable to create gzip reader: %v", err)
				return
			}
			defer gzipReader.Close()
			epgItem, err = parseXML(gzipReader)
			if err != nil {
				log.Error().Msgf("Unable to parse epg.xml.gzip FILE: %v", err)
				return
			}
		} else {
			epgItem, err = parseXML(bufReader)
			if err != nil {
				log.Error().Msgf("Unable to parse epg.xml FILE: %v", err)
				return
			}
		}

	} else {
		log.Info().Msg("Started parsing xml file...")
		// Open the XML file
		file, err := os.Open(epg.URL)
		if err != nil {
			log.Error().Msgf("Unable to get epg.xml FILE: %v", err)
			return
		}
		epgItem, err = parseXML(file)
		if err != nil {
			log.Error().Msgf("Unable to parse epg.xml FILE: %v", err)
			return
		}
		defer file.Close()
	}

	// Print out the parsed data
	for _, channel := range epgItem.Channels {
		if channel.ChannelId != "" {
			_, err := database.Db.GetEpgChannelByChannelId(ctx, channel.ChannelId)
			if err != nil {
				//log.Info().Msgf("EPG XML PARSER: Creating new channel: %s", channel.DisplayName)
				if _, err = database.Db.CreateEpgChannel(ctx, channel); err != nil {
					log.Error().Msgf("EPG XML PARSER: Failed to create new EPG channel: %v", err)
					continue
				}
			} else {
				//log.Info().Msgf("EPG XML PARSER: Channel already exists: %s", channel.DisplayName)
			}
		}

	}

	for _, programme := range epgItem.Programmes {
		//Convert times
		if !programme.Start.IsZero() {

			if programme.Channel != "" {
				FoundProg, err := database.Db.GetProgrammeByTime(ctx, programme.Channel, programme.Start.Time)
				if err != nil {
					//log.Info().Msgf("EPG XML PARSER: Creating new Programme: %s", programme.Title.Value)
					if _, err := database.Db.CreateEpgProgramme(ctx, programme); err != nil {
						log.Warn().Msgf("EPG XML PARSER: Failed to create new programme: %v", err)
						continue
					}
				} else {
					//log.Info().Msgf("EPG XML PARSER: Updating Programme: %s", programme.Title.Value)
					if err := database.Db.UpdateEpgProgramme(ctx, FoundProg.ID, &programme); err != nil {
						log.Warn().Msgf("EPG XML PARSER: Failed to update programme: %v", err)
						continue
					}
				}
			}
		} else {
			log.Error().Msg("EPG XML PARSER: NO PROGRAMMES FOUND")
		}

	}

	epg.UpdatedAt = time.Now()
	database.Db.UpdateEpg(ctx, epg.ID, epg)

	log.Info().Msg("EPG Parser Finished")

	// Get all templates.
	templates, err := database.Db.GetTemplates()
	if err != nil {
		log.Error().Msg("EPG XML PARSER: NO TEMPLATE FOUND")
	} else {
		for _, template := range *templates {
			go CreateEpgXML(template)
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
