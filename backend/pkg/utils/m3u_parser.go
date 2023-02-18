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

	log "github.com/sirupsen/logrus"
)

// M3uParser - A parser for m3u files.
type M3uParser struct {
	playlistID      int64
	Db              *database.Queries
	lines           []string
	content         string
	regexes         map[string]*regexp.Regexp
	matchedPlaylist int64
}

// ParseM3u - Parses the content of local file/URL.
func (m *M3uParser) ParseM3u(playlistID int64, path string) {
	m.playlistID = playlistID
	// Output to stdout instead of the default stderr
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.TextFormatter{TimestampFormat: "2006-01-02 15:04:05", FullTimestamp: true})
	// Only log the warning severity or above.
	log.SetLevel(log.InfoLevel)
	log.Infoln("Parser started")

	//Check if matching playlist exists
	m.matchedPlaylist = MatchDomain(m.Db, playlistID)

	m.regexes = make(map[string]*regexp.Regexp)
	m.regexes["file"] = CompileRegex(`(?m)^[a-zA-Z]:\\((?:.*?\\)*).*.[\d\w]{3,5}$|^(/[^/]*)+/?.[\d\w]{3,5}$`)
	//m.regexes["xuiID"] = CompileRegex(`xui-id="(.*?)"`)
	m.regexes["tvgID"] = CompileRegex(`tvg-id="(.*?)"`)
	m.regexes["tvgName"] = CompileRegex(`tvg-name="(.*?)"`)
	m.regexes["tvgLogo"] = CompileRegex(`tvg-logo="(.*?)"`)
	m.regexes["group"] = CompileRegex(`group-title="(.*?)"`)
	m.regexes["title"] = CompileRegex(`[,](.*?)$`)

	if isValidURL(path) {
		log.Infoln("Started parsing m3u URL...")
		resp, err := http.Get(path)
		if err != nil {
			log.Error("Unable to get M3U FILE: ", err)
			return
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Error("Unable to get M3U FILE: ", err)
			return
		}
		m.content = string(body)
	} else {
		log.Infoln("Started parsing m3u file...")
		body, err := os.ReadFile(path)
		if err != nil {
			log.Error("Unable to get M3U FILE: ", err)
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
		log.Infoln("No content to parse!!!")
	}
	log.Infoln("Parser finished")
}

func (m *M3uParser) parseLines() {
	re := CompileRegex("#EXTINF")

	for lineNumber := range m.lines {
		if re.Match([]byte(m.lines[lineNumber])) {
			m.parseLine(lineNumber)
		}
	}
}

func (m *M3uParser) parseLine(lineNumber int) {
	validate := NewValidator()
	playlistGroup := &models.PlaylistGroup{}
	playlistChannel := &models.PlaylistChannel{}
	channelURL := &models.ChannelUrl{}

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
		//title := getByRegex(m.regexes["title"], lineInfo)

		if tvgID != "" {
			playlistChannel.TvgID = tvgID
		}
		if tvgName != "" {
			playlistChannel.Name = tvgName
		}
		if tvgLogo != "" {
			playlistChannel.Logo = tvgLogo
		}

		if group != "" {
			// Checking, if playlist with given ID is exists.
			foundGroup, err := m.Db.GetPlGroupByName(group)
			if err != nil {
				log.Infoln("Group not found. Creating Group: ", group)
				playlistGroup.Name = group
				groupId, err := m.Db.CreatePlGroup(playlistGroup)
				if err != nil {
					playlistChannel.GroupID = groupId
				}
			} else {
				playlistChannel.GroupID = foundGroup.ID
			}
		}

		// Validate playlist fields.
		if err := validate.Struct(playlistChannel); err != nil {
			//Some fields are not valid.
			log.Warnln(err)
			return
		}

		//match playlist channels

		foundChannels, err := m.Db.GetPlChannelsByTvgID(tvgID)
		if err == nil {
			for _, foundChannel := range foundChannels {
				_, err := m.Db.ChannelUrlExists(m.matchedPlaylist, foundChannel.ID)
				if err != nil {
					log.Println(err)
					continue
				}
				log.Infoln("Channel found. Adding url to Channel: ", playlistChannel.Name)
				playlistChannel.UpdatedAt = time.Now()
				err = m.Db.UpdatePlChannel(foundChannel.ID, playlistChannel)
				if err != nil {
					log.Warnln(err)
					return
				}

				//Set channel URL model
				channelURL.Url = streamLink
				channelURL.PlaylistID = m.playlistID
				channelURL.PlaylistChannelId = foundChannel.ID
				channelURL.CreatedAt = time.Now()

				channelID, err := m.Db.ChannelUrlExists(m.playlistID, foundChannel.ID)
				if err != nil {
					err = m.Db.CreateChannelUrl(channelURL)
					if err != nil {
						log.Warnln(err)
					}
				} else {
					err = m.Db.UpdateChannelUrl(channelID, channelURL)
					if err != nil {
						log.Warnln(err)
					}
				}

				return
			}
		}
		log.Infoln("Channel not found. Creating Channel: ", playlistChannel.Name)
		playlistChannel.CreatedAt = time.Now()
		playlistChannelID, err := m.Db.CreatePlChannel(playlistChannel)
		if err != nil {
			log.Warnln(err)
			return
		}

		//Set channel URL model
		channelURL.Url = streamLink
		channelURL.PlaylistID = m.playlistID
		channelURL.PlaylistChannelId = playlistChannelID
		channelURL.CreatedAt = time.Now()
		err = m.Db.CreateChannelUrl(channelURL)
		if err != nil {
			log.Warnln(err)
			return
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
