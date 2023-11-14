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
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

type M3uTools struct {
	Db       *database.Queries
	template models.Template
	host     string
	port     int
}

func (m *M3uTools) CreateM3u(template models.Template) {
	m.host = settings.APP_SETTINGS.Server.Host
	m.port = settings.APP_SETTINGS.Server.Port
	m.template = template

	reader, err := m.marshall()
	if err != nil {
		log.Err(err)
	}
	b := reader.(*bytes.Buffer)

	if err := os.WriteFile(fmt.Sprintf("%s/%s.m3u", settings.M3U_FILEPATH, template.Name), b.Bytes(), os.ModePerm); err != nil {
		fmt.Println("Error writing file:", err)
		return
	}

	go CreateEpgXML(m.Db, template)
}

func (m *M3uTools) RenameTemplate(template *models.Template, oldTemplate string) {
	var filtered string
	oldfile := fmt.Sprintf("%s/%s.m3u", settings.M3U_FILEPATH, oldTemplate)
	newfile := fmt.Sprintf("%s/%s.m3u", settings.M3U_FILEPATH, template.Name)

	// Read the content of the XML file into a byte array.
	content, err := os.ReadFile(oldfile)
	if err != nil {
		log.Error().Msgf("Error reading file: %v", err)
		return
	}

	oldxmltvURL := fmt.Sprintf("http://%s:%d/xmltv/%s.xml", m.host, m.port, oldTemplate)
	xmltvURL := fmt.Sprintf("http://%s:%d/xmltv/%s.xml", m.host, m.port, template.Name)

	filter := fmt.Sprintf("#EXTM3U url-tvg=\"%s\" x-tvg-url=\"%s\"\n", xmltvURL, oldxmltvURL)
	newxml := fmt.Sprintf("#EXTM3U url-tvg=\"%s\" x-tvg-url=\"%s\"\n", xmltvURL, xmltvURL)
	lines := strings.Split(string(content), "\n")

	// Loop through each line and check if it contains the group.
	for _, line := range lines {
		if strings.Contains(line, filter) {
			strings.Replace(line, filter, newxml, 0)
		}
		filtered += line + "\n"
	}

	// Write the filtered content back to the original file.
	if err = os.WriteFile(newfile, []byte(filtered), 0644); err != nil {
		fmt.Println("Error writing file:", err)
		return
	}

	if err := os.Remove(oldfile); err != nil {
		fmt.Println("Error removing file:", err)
		return
	}

	go CreateEpgXML(m.Db, *template)
}

func (m *M3uTools) RemoveTemplate(template *models.Template) {
	file := fmt.Sprintf("%s/%s.m3u", settings.M3U_FILEPATH, template.Name)
	if err := os.Remove(file); err != nil {
		fmt.Println("Error removing file:", err)
		return
	}
}

func (m *M3uTools) UpdateGroup(template *models.Template, group *models.TemplateGroup, oldGroup string) {
	// Create a new string variable to hold the filtered content.
	var filtered string

	file := fmt.Sprintf("%s/%s.m3u", settings.M3U_FILEPATH, template.Name)

	// Read the content of the XML file into a byte array.
	content, err := os.ReadFile(file)
	if err != nil {
		log.Error().Msgf("Error reading file: %v", err)
		return
	}

	filter := fmt.Sprintf("group-title=\"%s\"", oldGroup)
	newGroup := fmt.Sprintf("group-title=\"%s\"", group.Name)
	lines := strings.Split(string(content), "\n")

	// Loop through each line and check if it contains the group.
	for _, line := range lines {
		if strings.Contains(line, filter) {
			strings.Replace(line, filter, newGroup, 0)
		}
		filtered += line + "\n"
	}

	// Write the filtered content back to the original file.
	if err = os.WriteFile(file, []byte(filtered), 0644); err != nil {
		fmt.Println("Error writing file:", err)
		return
	}
}

func (m *M3uTools) AddChannel(templateChannel *models.TemplateChannel, groupId int64) {
	//TODO ADD CHANNEL
}

func (m *M3uTools) UpdateChannel(template *models.Template, channel *models.TemplateChannelLogo, oldChannel *models.TemplateChannel) {
	// Create a new string variable to hold the filtered content.
	var filtered string

	file := fmt.Sprintf("%s/%s.m3u", settings.M3U_FILEPATH, template.Name)

	// Read the content of the XML file into a byte array.
	content, err := os.ReadFile(file)
	if err != nil {
		log.Error().Msgf("Error reading file: %v", err)
		return
	}

	logo, err := m.Db.GetLogo(oldChannel.LogoId)
	if err != nil {
		log.Warn().Msg(err.Error())
		return
	}
	oldLogoURL := fmt.Sprintf("http://%s:%d/%s", m.host, m.port, GetLogoUrl(logo.Uuid))
	logoURL := fmt.Sprintf("http://%s:%d/%s", m.host, m.port, channel.Logo)

	// TODO FIX CHANNEL NAME IN M3U
	filter := fmt.Sprintf("tvg-name=\"%s\" tvg-id=\"%s\" tvg-logo=\"%s\"", oldChannel.TvgID, oldChannel.TvgID, oldLogoURL)
	newChannel := fmt.Sprintf("tvg-name=\"%s\" tvg-id=\"%s\" tvg-logo=\"%s\"", channel.TvgID, channel.TvgID, logoURL)
	lines := strings.Split(string(content), "\n")

	// Loop through each line and check if it contains the group.
	for _, line := range lines {
		if strings.Contains(line, filter) {
			strings.Replace(line, filter, newChannel, 0)
		}
		filtered += line + "\n"
	}

	// Write the filtered content back to the original file.
	if err = os.WriteFile(file, []byte(filtered), 0644); err != nil {
		fmt.Println("Error writing file:", err)
		return
	}

	go CreateEpgXML(m.Db, *template)
}

func (m *M3uTools) RemoveGroup(template *models.Template, group *models.TemplateGroup) {
	// Create a new string variable to hold the filtered content.
	var filtered string

	file := fmt.Sprintf("%s/%s.m3u", settings.M3U_FILEPATH, template.Name)

	// Read the content of the XML file into a byte array.
	content, err := os.ReadFile(file)
	if err != nil {
		log.Error().Msgf("Error reading file: %v", err)
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
	if err = os.WriteFile(file, []byte(filtered), 0644); err != nil {
		fmt.Println("Error writing file:", err)
		return
	}
}

func (m *M3uTools) RemoveChannel(template *models.Template, group *models.TemplateGroup, channel *models.TemplateChannel) {
	// Create a new string variable to hold the filtered content.
	var filtered string

	file := fmt.Sprintf("%s/%s.m3u", settings.M3U_FILEPATH, template.Name)

	// Read the content of the XML file into a byte array.
	content, err := os.ReadFile(file)
	if err != nil {
		log.Error().Msgf("Error reading file: %v", err)
		return
	}

	filterGroup := fmt.Sprintf("group-title=\"%s\"", group.Name)
	filterChannel := fmt.Sprintf("tvg-name=\"%s\"", channel.Name)
	lines := strings.Split(string(content), "\n")

	// Loop through each line and check if it contains the group & channel.
	var skipUrl bool
	for _, line := range lines {
		if skipUrl {
			// If the flag is set, skip the next line, the url
			skipUrl = false
			continue
		} else if strings.Contains(line, filterGroup) && strings.Contains(line, filterChannel) {
			// If the line contains the group, set the flag to skip the next two lines.
			skipUrl = true
			continue
		}
		// If the line does not contain group & channel , append it to the filtered content.
		filtered += line + "\n"
	}

	// Write the filtered content back to the original file.
	if err = os.WriteFile(file, []byte(filtered), 0644); err != nil {
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
		log.Err(err)
		return nil
	}

	tmplGroupChannels, err := m.Db.GetTmplGroupChannelsByTmpl(m.template.ID)
	if err != nil {
		log.Warn().Msg(err.Error())
		return err
	}

	for _, item := range tmplGroupChannels {
		group, err := m.Db.GetTmplGroup(item.GroupId)
		if err != nil {
			log.Warn().Msg(err.Error())
			continue
		}
		log.Printf("M3U Creation: Found Template Group: %s", group.Name)
		channel, err := m.Db.GetTmplChannel(item.ChannelId)
		if err != nil {
			log.Warn().Msg(err.Error())
			continue
		}
		logo, err := m.Db.GetLogo(channel.LogoId)
		if err != nil {
			log.Warn().Msg(err.Error())
			continue
		}
		logoURL := fmt.Sprintf("http://%s:%d/%s", m.host, m.port, GetLogoUrl(logo.Uuid))

		log.Printf("M3U Creation: Adding Template Channel: %s", channel.Name)
		channelURL := fmt.Sprintf("http://%s:%d/stream/%s", m.host, m.port, channel.Uuid)
		_, err = writer.WriteString(fmt.Sprintf("#EXTINF:-1 tvg-chno=\"%d\" tvg-name=\"%s\" tvg-id=\"%s\" tvg-logo=\"%s\" group-title=\"%s\",%s\n%s\n", chNo, channel.TvgID, channel.TvgID, logoURL, group.Name, channel.Name, channelURL))
		if err != nil {
			log.Err(err)
			continue
		}
		chNo++
	}

	return writer.Flush()
}
