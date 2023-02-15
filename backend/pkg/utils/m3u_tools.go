package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	log "github.com/sirupsen/logrus"
)

var host string
var port int

type M3uTools struct {
	Db        *database.Queries
	tmplItems []models.TemplateItem
	template  models.Template
}

func (m *M3uTools) CreateM3u(template models.Template) {
	host = "127.0.0.1"
	port = 8000
	m.template = template

	tmplItems, err := m.Db.GetTmplItems(template.ID)
	if err != nil {
		log.Warnln(err)
		return
	}
	m.tmplItems = tmplItems

	reader, err := m.marshall()
	if err != nil {
		log.Error(err)
	}
	b := reader.(*bytes.Buffer)

	os.WriteFile(fmt.Sprintf("./configs/stream/%s.m3u", template.Name), b.Bytes(), os.ModePerm)

}

func (m *M3uTools) RemoveM3uItems(templateItem models.TemplateItem) {
	// Create a new string variable to hold the filtered content.
	var filtered string

	template, err := m.Db.GetTemplate(templateItem.TemplateId)
	if err != nil {
		log.Error("Error finding Template: ", err)
		return
	}
	file := fmt.Sprintf("./configs/stream/%s.m3u", template.Name)

	// Read the content of the XML file into a byte array.
	content, err := os.ReadFile(file)
	if err != nil {
		log.Error("Error reading file:", err)
		return
	}

	group, err := m.Db.GetTmplGroup(templateItem.GroupId)
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

	xmltvURL := fmt.Sprintf("http://%s:%d/xmltv/%s.xml", host, port, m.template.Name)

	_, err := writer.WriteString(fmt.Sprintf("#EXTM3U url-tvg=\"%s\" x-tvg-url=\"%s\"\n", xmltvURL, xmltvURL))
	if err != nil {
		log.Error(err)
		return nil
	}

	for _, item := range m.tmplItems {
		group, err := m.Db.GetTmplGroup(item.GroupId)
		if err != nil {
			log.Warnln(err)
			continue
		}
		log.Println("M3U Creation: Found Template Group: ", group.Name)
		tmplGroupItems, err := m.Db.GetTmplGroupItems(group.ID)
		if err != nil {
			log.Warnln(err)
			continue
		}
		for _, groupItem := range tmplGroupItems {
			channel, err := m.Db.GetTmplChannel(groupItem.ChannelId)
			if err != nil {
				log.Warnln(err)
				continue
			}
			log.Println("M3U Creation: Adding Template Channel: ", channel.Name)
			channelURL := fmt.Sprintf("http://%s:%d/stream/%s", host, port, channel.Uuid)
			_, err = writer.WriteString(fmt.Sprintf("#EXTINF:-1 tvg-chno=\"%d\" tvg-name=\"%s\" tvg-id=\"%s\" tvg-logo=\"%s\" group-title=\"%s\",%s\n%s\n", chNo, channel.Name, channel.TvgID, channel.Logo, group.Name, channel.Name, channelURL))
			if err != nil {
				log.Error(err)
				continue
			}
			chNo++
		}
	}

	return writer.Flush()
}
