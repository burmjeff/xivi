package utils

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
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
	//m.regexes["xuiID"] = CompileRegex(`xui-id="(.*?)"`)
	m.regexes["tvgID"] = CompileRegex(`tvg-id="(.*?)"`)
	m.regexes["tvgName"] = CompileRegex(`tvg-name="(.*?)"`)
	m.regexes["tvgLogo"] = CompileRegex(`tvg-logo="(.*?)"`)
	m.regexes["group"] = CompileRegex(`group-title="(.*?)"`)
	m.regexes["title"] = CompileRegex(`[,](.*?)$`)

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
	vectorIn := make(chan models.PlaylistChannel)
	go PlaylistVectorQueue(vectorIn)

	re := CompileRegex("#EXTINF")

	for lineNumber := range m.lines {
		if re.Match([]byte(m.lines[lineNumber])) {
			m.parseLine(lineNumber, vectorIn)
		}
	}
	close(vectorIn)
}

func (m *M3uParser) parseLine(lineNumber int, vectorIn chan models.PlaylistChannel) {
	validate := NewValidator()
	playlistGroup := models.PlaylistGroup{}
	playlistChannel := models.PlaylistChannel{}
	channelURL := models.ChannelUrl{}

	var streamLink string
	//channel := make(Channel)
	lineInfo := m.lines[lineNumber]

	for i := range [2]int{1, 2} {
		isUrl := isValidURL(m.lines[lineNumber+i])
		if isUrl {
			streamLink = m.lines[lineNumber+i]
			break
		}
	}

	if lineInfo != "" && streamLink != "" {

		//xuiID := GetByRegex(m.regexes["xuiID"], lineInfo)
		tvgID := GetByRegex(m.regexes["tvgID"], lineInfo)
		tvgName := GetByRegex(m.regexes["tvgName"], lineInfo)
		tvgLogo := GetByRegex(m.regexes["tvgLogo"], lineInfo)
		group := GetByRegex(m.regexes["group"], lineInfo)
		title := GetByRegex(m.regexes["title"], lineInfo)

		if tvgID != "" {
			playlistChannel.TvgID = &tvgID
			//Add new channel vectors
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

		var groupID int64 = 0
		if group != "" {
			// Checking, if playlist with given ID is exists.
			if foundId, err := database.Db.GetPlGroupByName(group); foundId == 0 {
				log.Info().Msgf("Group not found. %v, Creating Group: %s", err, group)
				playlistGroup.Name = group
				if groupID, err = database.Db.CreatePlGroup(playlistGroup); groupID == 0 {
					log.Error().Msgf("FAILED TO CREATE PLAYLIST GROUP: %v", err)
					return
				} else {
					if _, err := database.Db.CreatePlGroupItem(m.playlistID, groupID); err != nil {
						log.Error().Msgf("FAILED TO CREATE PLAYLIST_GROUP_ITEM: %v", err)
					}
				}
			} else {
				groupID = foundId
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
		foundChannels, err := database.Db.GetM3UParseByTvgID(tvgID, groupID, m.playlistID)
		if err != nil {
			log.Error().Err(err)
		}

		if len(foundChannels) == 0 {
			log.Info().Msgf("Channel not found. Creating Channel: %s", playlistChannel.Title)
			playlistChannel.CreatedAt = time.Now()
			playlistChannelID, err := database.Db.CreatePlChannel(playlistChannel)
			if err != nil {
				log.Warn().Msg(err.Error())
				return
			}
			playlistChannel.ID = playlistChannelID
			m.createPlaylistGroupChannel(groupID, playlistChannel)
			vectorIn <- playlistChannel

			//Set channel URL model
			channelURL.Url = streamLink
			channelURL.PlaylistID = m.playlistID
			channelURL.PlaylistChannelId = playlistChannel.ID
			channelURL.CreatedAt = time.Now()
			err = database.Db.CreateChannelUrl(channelURL)
			if err != nil {
				log.Warn().Msg(err.Error())
				return
			}
		} else {
			for _, foundChannel := range foundChannels {
				log.Info().Msgf("Channel found. Adding url to Channel: %s", playlistChannel.Title)
				playlistChannel.UpdatedAt = time.Now()
				err = database.Db.UpdatePlChannel(foundChannel.ID, playlistChannel)
				if err != nil {
					log.Warn().Msg(err.Error())
					return
				}
				playlistChannel.ID = foundChannel.ID
				if foundChannel.Title != playlistChannel.Title {
					vectorIn <- playlistChannel
				}

				//Set channel URL model
				channelURL.Url = streamLink
				channelURL.PlaylistID = m.playlistID
				channelURL.PlaylistChannelId = foundChannel.ID

				channelID, err := database.Db.ChannelUrlExists(m.playlistID, foundChannel.ID)
				if err != nil {
					channelURL.CreatedAt = time.Now()
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
	}
}

func (m *M3uParser) createPlaylistGroupChannel(groupID int64, playlistChannel models.PlaylistChannel) {
	_, err := database.Db.CreatePlGroupChannel(groupID, playlistChannel.ID)
	if err != nil {
		log.Debug().Msgf("FAILED TO CREATE PLAYLIST_GROUP_CHANNEL: %v", err)
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
