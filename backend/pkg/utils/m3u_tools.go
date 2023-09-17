package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	log "github.com/sirupsen/logrus"
)

type M3uTools struct {
	Db       *database.Queries
	template models.Template
	host     string
	port     int
}

func (m *M3uTools) CreateM3u(template models.Template) {
	m.host = os.Getenv("STREAMING_HOST")
	port, err := strconv.Atoi(os.Getenv("STREAMING_PORT"))
	if err != nil {
		log.Error("Not a valid Streaming Port: ", os.Getenv("STREAMING_PORT"))
		return
	}
	m.port = port
	m.template = template

	reader, err := m.marshall()
	if err != nil {
		log.Error(err)
	}
	b := reader.(*bytes.Buffer)

	os.WriteFile(fmt.Sprintf("./configs/stream/%s.m3u", template.Name), b.Bytes(), os.ModePerm)

}

func (m *M3uTools) RemoveM3uItems(templateGroupItem *models.TemplateGroupItem) {
	// Create a new string variable to hold the filtered content.
	var filtered string

	template, err := m.Db.GetTemplate(templateGroupItem.TemplateId)
	if err != nil {
		log.Error("Error finding Template: ", err)
		return
	}
	file := fmt.Sprintf("%s/m3u/%s.m3u", os.Getenv("STREAM_PATH"), template.Name)

	// Read the content of the XML file into a byte array.
	content, err := os.ReadFile(file)
	if err != nil {
		log.Error("Error reading file:", err)
		return
	}

	group, err := m.Db.GetTmplGroup(templateGroupItem.GroupId)
	if err != nil {
		log.Error("Error finding Group: ", err)
		return
	}

	filter := fmt.Sprintf("group-title=\"%s\"", group.Name)
	lines := strings.Split(string(content), "\n")

	// Loop through each line and check if it contains the group.
	var skipUrl bool
	for _, line := range lines {
		if skipUrl {
			// If the flag is set, skip the next line, the url
			skipUrl = false
			continue
		} else if strings.Contains(line, filter) {
			// If the line contains the group, set the flag to skip the next two lines.
			skipUrl = true
			continue
		}
		// If the line does not contain group , append it to the filtered content.
		filtered += line + "\n"
	}

	// Write the filtered content back to the original file.
	err = os.WriteFile(file, []byte(filtered), 0644)
	if err != nil {
		fmt.Println("Error writing file:", err)
		return
	}
}

func (m *M3uTools) marshall() (io.Reader, error) {
	buf := new(bytes.Buffer)
	w := bufio.NewWriter(buf)
	if err := m.marshallInto(w); err != nil {
		return nil, err
	}

	return buf, nil
}

func (m *M3uTools) marshallInto(writer *bufio.Writer) error {
	chNo := 1

	xmltvURL := fmt.Sprintf("http://%s:%d/xmltv/%s.xml", m.host, m.port, m.template.Name)

	_, err := writer.WriteString(fmt.Sprintf("#EXTM3U url-tvg=\"%s\" x-tvg-url=\"%s\"\n", xmltvURL, xmltvURL))
	if err != nil {
		log.Error(err)
		return nil
	}

	tmplGroupChannels, err := m.Db.GetTmplGroupChannels(m.template.ID)
	if err != nil {
		log.Warnln(err)
		return err
	}

	for _, item := range tmplGroupChannels {
		group, err := m.Db.GetTmplGroup(item.GroupId)
		if err != nil {
			log.Warnln(err)
			continue
		}
		log.Println("M3U Creation: Found Template Group: ", group.Name)
		channel, err := m.Db.GetTmplChannel(item.ChannelId)
		if err != nil {
			log.Warnln(err)
			continue
		}
		logo := getChannelLogo(channel.LogoId)

		log.Println("M3U Creation: Adding Template Channel: ", channel.Name)
		channelURL := fmt.Sprintf("http://%s:%d/stream/%s", m.host, m.port, channel.Uuid)
		_, err = writer.WriteString(fmt.Sprintf("#EXTINF:-1 tvg-chno=\"%d\" tvg-name=\"%s\" tvg-id=\"%s\" tvg-logo=\"%s\" group-title=\"%s\",%s\n%s\n", chNo, channel.Name, channel.TvgID, logo, group.Name, channel.Name, channelURL))
		if err != nil {
			log.Error(err)
			continue
		}
		chNo++
	}

	return writer.Flush()
}
