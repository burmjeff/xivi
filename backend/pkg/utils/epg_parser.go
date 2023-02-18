package utils

import (
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	log "github.com/sirupsen/logrus"
)

// EpgParser - A parser for epg xml files.
type EpgParser struct {
	Db *database.Queries
}

type EPG struct {
	Channels   []models.EpgChannel   `xml:"channel"`
	Programmes []models.EpgProgramme `xml:"programme"`
}

func (m *EpgParser) ParseEpg(playlistID int64, path string) {
	localTime, err := time.LoadLocation(os.Getenv("TZ"))
	if err != nil {
		log.Fatal(err)
	}
	epg := EPG{}

	// Output to stdout instead of the default stderr
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.TextFormatter{TimestampFormat: "2006-01-02 15:04:05", FullTimestamp: true})
	// Only log the warning severity or above.
	log.SetLevel(log.InfoLevel)
	log.Infoln("Parser started")

	if isValidURL(path) {
		log.Infoln("Started parsing xml URL...")
		resp, err := http.Get(path)
		if err != nil {
			log.Error("Unable to get epg.xml FILE: ", err)
			return
		}
		epg, err = parseXML(resp.Body)
		if err != nil {
			log.Error("Unable to parse epg.xml FILE: ", err)
			return
		}
		defer resp.Body.Close()
	} else {
		log.Infoln("Started parsing xml file...")
		// Open the XML file
		file, err := os.Open(path)
		if err != nil {
			log.Error("Unable to get epg.xml FILE: ", err)
			return
		}
		epg, err = parseXML(file)
		if err != nil {
			log.Error("Unable to parse epg.xml FILE: ", err)
			return
		}
		defer file.Close()
	}

	// Decode the XML file into a struct
	/*
		for {
			t, err := m.decoder.Token()
			if err != nil {
				fmt.Println("Error parsing XML:", err)
				return
			}

			switch se := t.(type) {
			case xml.StartElement:
				if se.Name.Local == "channel" {
					var channel models.EpgChannel
					m.decoder.DecodeElement(&channel, &se)
					epg.Channels = append(epg.Channels, channel)
				} else if se.Name.Local == "programme" {
					var programme models.EpgProgramme
					m.decoder.DecodeElement(&programme, &se)
					epg.Programmes = append(epg.Programmes, programme)
				}
			}
			if _, ok := t.(xml.EndElement); ok {
				break
			}
		} */

	// Print out the parsed data
	for _, channel := range epg.Channels {
		if channel.ChannelId != "" {
			_, err := m.Db.GetEpgChannelByChannelId(channel.ChannelId)
			if err != nil {
				log.Info("EPG XML PARSER: Creating new channel: ", channel.DisplayName)
				_, err = m.Db.CreateEpgChannel(&channel)
				if err != nil {
					log.Error("EPG XML PARSER: Failed to create new EPG channel: ", err)
					continue
				}
			} else {
				log.Info("EPG XML PARSER: Channel already exists: ", channel.DisplayName)
			}
		}
	}

	for _, programme := range epg.Programmes {
		//Convert times
		if programme.StartString != "" {
			startTime, err := time.Parse("20060102150405 -0700", programme.StartString)
			if err != nil {
				log.Error("EPG XML PARSER: Failed to convert Start time: ", err)
				continue
			}
			programme.Start = startTime.In(localTime)
			stopTime, err := time.Parse("20060102150405 -0700", programme.StopString)
			if err != nil {
				log.Error("EPG XML PARSER: Failed to convert Stop time: ", err)
				continue
			}
			programme.Stop = stopTime.In(localTime)

			if programme.Channel != "" {
				FoundProg, err := m.Db.GetEpgProgrammeByChannelandTime(programme.Channel, programme.Start)
				if err != nil {
					log.Info("EPG XML PARSER: Creating new Programme: ", programme.Title.Value)
					m.Db.CreateEpgProgramme(&programme)
					if err != nil {
						log.Warn("EPG XML PARSER: Failed to create new programme: ", err)
						continue
					}
				} else {
					log.Info("EPG XML PARSER: Updating Programme: ", programme.Title.Value)
					m.Db.UpdateEpgProgramme(FoundProg.ID, &programme)
					if err != nil {
						log.Warn("EPG XML PARSER: Failed to update programme", err)
						continue
					}
				}
			}
		} else {
			log.Error("EPG XML PARSER: NO PROGRAMMES FOUND")
		}

	}

}

func parseXML(xmlData io.Reader) (EPG, error) {
	var epg EPG
	decoder := xml.NewDecoder(xmlData)

	for {
		// Read tokens from the XML data stream
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return EPG{}, err
		}

		// If the token is a start element, check the element name and decode the corresponding struct
		if se, ok := token.(xml.StartElement); ok {
			switch se.Name.Local {
			case "channel":
				var channel models.EpgChannel
				if err := decoder.DecodeElement(&channel, &se); err != nil {
					return EPG{}, err
				}
				epg.Channels = append(epg.Channels, channel)
			case "programme":
				var programme models.EpgProgramme
				if err := decoder.DecodeElement(&programme, &se); err != nil {
					return EPG{}, err
				}
				epg.Programmes = append(epg.Programmes, programme)
			}
		}
	}

	return epg, nil
}
