package utils

import (
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/rs/zerolog/log"
)

// M3uParser - A parser for m3u files.
type M3uParser struct {
	playlistID      int64
	lines           []string
	content         string
	regexes         map[string]*regexp.Regexp
	matchedPlaylist int64
}

// ParseM3u - Parses the content of local file/URL.
func (m *M3uParser) ParseM3u(playlist models.Playlist) {
	m.playlistID = playlist.ID
	log.Info().Msg("Parser started")

	//Check if matching playlist exists
	m.matchedPlaylist = MatchDomain(playlist.ID)

	m.regexes = make(map[string]*regexp.Regexp)
	m.regexes["file"] = CompileRegex(`(?m)^[a-zA-Z]:\\((?:.*?\\)*).*.[\d\w]{3,5}$|^(/[^/]*)+/?.[\d\w]{3,5}$`)
	m.regexes["xuiID"] = CompileRegex(`xui-id="(.*?)"`)
	m.regexes["tvgID"] = CompileRegex(`tvg-id="(.*?)"`)
	m.regexes["tvgName"] = CompileRegex(`tvg-name="(.*?)"`)
	m.regexes["tvgLogo"] = CompileRegex(`tvg-logo="(.*?)"`)
	m.regexes["group"] = CompileRegex(`group-title="(.*?)"`)
	m.regexes["title"] = CompileRegex(`(?:",|" ,)(.*?)$`)

	if isValidURL(playlist.URL) {
		log.Info().Msg("Started parsing m3u URL...")
		resp, err := http.Get(playlist.URL)
		if err != nil {
			log.Error().Msgf("Unable to get M3U FILE: %v", err)
			return
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Error().Msgf("Unable to get M3U FILE: %v", err)
			return
		}
		m.content = string(body)
	} else {
		log.Info().Msg("Started parsing m3u file...")
		body, err := os.ReadFile(playlist.URL)
		if err != nil {
			log.Error().Msgf("Unable to get M3U FILE: %v", err)
			return
		}
		m.content = string(body)
	}

	if m.content != "" {
		for _, line := range strings.Split(m.content, "\n") {
			if strings.TrimSpace(line) != "" {
				m.lines = append(m.lines, strings.TrimSpace(line))
			}
		}
	}
	if len(m.lines) > 0 {
		m.parseLines()
	} else {
		log.Info().Msg("No content to parse!!!")
	}

	playlist.UpdatedAt = time.Now()
	database.Db.UpdatePlaylist(playlist.ID, &playlist)

	log.Info().Msg("Parser finished")
}

func (m *M3uParser) parseLines() {
	chunkSize := 10
	var wg sync.WaitGroup
	vectorIn := make(chan models.PlaylistChannel)
	go PlaylistVectorQueue(vectorIn)

	re := CompileRegex("#EXTINF")

	var chunks [][]string
	if len(m.lines) > 100 {
		chunkSize = int(math.RoundToEven(float64(len(m.lines) / 10)))
	}

	for i := 0; i < len(m.lines); i += chunkSize {
		end := i + chunkSize

		if end >= len(m.lines) {
			end = len(m.lines)
		} else if re.Match([]byte(m.lines[end])) {
			end += 1
		}

		chunks = append(chunks, m.lines[i:end])
	}

	for _, chunk := range chunks {
		wg.Add(1)
		go func(chunk []string) {
			defer wg.Done()
			for i := 0; i < len(chunk); i += 1 {
				if re.Match([]byte(chunk[i])) {
					if i+1 < len(chunk) && isValidURL(chunk[i+1]) {
						m.parseLine(chunk[i], chunk[i+1], vectorIn)
					}
				}
			}
		}(chunk)
	}

	wg.Wait()
	close(vectorIn)
}

func (m *M3uParser) parseLine(line string, streamLink string, vectorIn chan models.PlaylistChannel) {
	validate := NewValidator()
	playlistGroup := models.PlaylistGroup{}
	playlistGroup.PlaylistId = m.playlistID
	playlistChannel := models.PlaylistChannel{}
	channelURL := models.ChannelUrl{}

	if line != "" && streamLink != "" {

		tvgID := GetByRegex(m.regexes["tvgID"], line)
		tvgName := GetByRegex(m.regexes["tvgName"], line)
		tvgLogo := GetByRegex(m.regexes["tvgLogo"], line)
		groupName := GetByRegex(m.regexes["group"], line)
		title := GetByRegex(m.regexes["title"], line)

		if tvgID != "" {
			playlistChannel.TvgID = &tvgID
			//Add new channel vectors
		} else {
			tvgID = GetByRegex(m.regexes["xuiID"], line)
			if tvgID != "" {
				playlistChannel.TvgID = &tvgID
			}
		}
		if tvgName != "" {
			playlistChannel.TvgName = tvgName
		}
		if tvgLogo != "" {
			playlistChannel.Logo = &tvgLogo
		}

		if title != "" {
			playlistChannel.Title = title
			if tvgName == "" {
				playlistChannel.TvgName = title
			}
		} else if tvgName != "" {
			playlistChannel.Title = tvgName
		} else if tvgID != "" {
			playlistChannel.Title = tvgID
		}

		if groupName != "" {
			// Checking, if playlist with given ID is exists.
			if group, err := database.Db.GetPlGroupByName(m.playlistID, groupName); group == nil {
				log.Info().Msgf("Group not found. %v, Creating Group: %s", err, groupName)
				playlistGroup.Name = groupName
				if groupId, err := database.Db.CreatePlGroup(playlistGroup); err != nil {
					log.Error().Msgf("FAILED TO CREATE PLAYLIST GROUP: %v", err)
					return
				} else {
					playlistChannel.GroupId = groupId
				}
			} else {
				playlistChannel.GroupId = group.ID
				if group.Enabled == false {
					return
				}

			}
		} else {
			log.Error().Msgf("M3U PARSER: No Group found for %s", title)
			return
		}

		// Validate playlist fields.
		if err := validate.Struct(playlistChannel); err != nil {
			//Some fields are not valid.
			log.Error().Msg(err.Error())
			return
		}

		//match playlist channels
		foundChannels, err := database.Db.GetM3UParseByTvgID(tvgID, playlistChannel.GroupId, m.playlistID)
		if err != nil {
			log.Error().Msgf("M3U_PARSER, tvg_id not found: %v", err)
			if foundChannels, err = database.Db.GetM3UParseByTvgName(tvgName, playlistChannel.GroupId, m.playlistID); err != nil {
				log.Error().Msgf("M3U_PARSER, tvg_name not found: %v", err)
				if foundChannels, err = database.Db.GetM3UParseByTitle(title, playlistChannel.GroupId, m.playlistID); err != nil {
					log.Error().Msgf("M3U_PARSER, title not found: %v", err)
				}
			}
		}

		if len(foundChannels) == 0 {
			//log.Info().Msgf("Channel not found. Creating Channel: %s", playlistChannel.Title)
			playlistChannel.CreatedAt = time.Now()
			playlistChannel.UpdatedAt = time.Now()
			playlistChannelID, err := database.Db.CreatePlChannel(playlistChannel)
			if err != nil {
				log.Warn().Msg(err.Error())
				return
			}
			playlistChannel.ID = playlistChannelID
			vectorIn <- playlistChannel

			//Set channel URL model
			channelURL.Url = streamLink
			channelURL.ChannelId = playlistChannel.ID
			channelURL.CreatedAt = time.Now()
			channelURL.UpdatedAt = time.Now()
			err = database.Db.CreateChannelUrl(channelURL)
			if err != nil {
				log.Warn().Msg(err.Error())
				return
			}
		} else {
			for _, foundChannel := range foundChannels {
				//log.Info().Msgf("Channel found. Adding url to Channel: %s", playlistChannel.Title)
				playlistChannel.UpdatedAt = time.Now()
				playlistChannel.Enabled = foundChannel.Enabled
				err = database.Db.UpdatePlChannel(foundChannel.ID, playlistChannel)
				if err != nil {
					log.Warn().Msg(err.Error())
					return
				}
				playlistChannel.ID = foundChannel.ID
				vectorIn <- playlistChannel

				//Set channel URL model
				channelURL.Url = streamLink
				channelURL.ChannelId = foundChannel.ID

				channelID, err := database.Db.ChannelUrlExists(m.playlistID, foundChannel.ID)
				if err != nil {
					channelURL.CreatedAt = time.Now()
					channelURL.UpdatedAt = time.Now()
					err = database.Db.CreateChannelUrl(channelURL)
					if err != nil {
						log.Warn().Msg(err.Error())
						continue
					}
				} else {
					channelURL.UpdatedAt = time.Now()
					err = database.Db.UpdateChannelUrl(channelID, channelURL)
					if err != nil {
						log.Warn().Msg(err.Error())
						continue
					}
				}
			}
		}
		if playlistChannel.TvgID == nil {
			tvgid := strconv.Itoa(int(playlistChannel.ID))
			playlistChannel.TvgID = &tvgid
			err = database.Db.UpdatePlChannel(playlistChannel.ID, playlistChannel)
			if err != nil {
				log.Warn().Msg(err.Error())
				return
			}

		}
	}
}

func isValidURL(toTest string) bool {
	_, err := url.ParseRequestURI(toTest)
	if err != nil {
		return false
	}

	u, err := url.Parse(toTest)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	return true
}
