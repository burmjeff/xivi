package utils

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

type M3uTools struct {
	template models.Template
	host     string
	port     int
	// File cache to reduce I/O operations
	fileCache    map[string][]string
	fileCacheMux sync.RWMutex
}

func internalM3UPath(lineupName string) string {
	sum := sha256.Sum256([]byte(lineupName))
	return fmt.Sprintf("%s/internal-%s.m3u", settings.M3U_FILEPATH, hex.EncodeToString(sum[:12]))
}

// NewM3uTools creates a new instance with initialized cache
func NewM3uTools() *M3uTools {
	return &M3uTools{
		host:      settings.Current().Server.Host,
		port:      settings.Current().Server.Port,
		fileCache: make(map[string][]string),
	}
}

func (m *M3uTools) CreateM3u(template models.Template) error {
	m.host = settings.Current().Server.Host
	m.port = settings.Current().Server.Port
	m.template = template

	reader, err := m.marshall()
	if err != nil {
		log.Error().Err(err).Str("template", template.Name).Msg("Failed to build M3U")
		return fmt.Errorf("could not build the M3U export: %w", err)
	}

	b, ok := reader.(*bytes.Buffer)
	if !ok {
		return fmt.Errorf("could not build the M3U export: unexpected buffer type %T", reader)
	}
	if err := os.MkdirAll(settings.M3U_FILEPATH, 0700); err != nil {
		log.Error().Err(err).Msg("Failed to create private M3U directory")
		return fmt.Errorf("could not create the M3U output directory: %w", err)
	}
	filePath := internalM3UPath(template.Name)

	// Write to a temporary file so a failed publish cannot corrupt the last good export.
	f, err := os.CreateTemp(settings.M3U_FILEPATH, ".xivi-*.m3u.tmp")
	if err != nil {
		log.Error().Err(err).Msg("Failed to create temporary M3U file")
		return fmt.Errorf("could not create the M3U export: %w", err)
	}
	tempFilePath := f.Name()
	defer func() {
		_ = f.Close()
		_ = os.Remove(tempFilePath)
	}()
	if err := f.Chmod(0600); err != nil {
		return fmt.Errorf("could not set M3U file permissions: %w", err)
	}

	w := bufio.NewWriter(f)
	if _, err := w.Write(b.Bytes()); err != nil {
		log.Error().Err(err).Msg("Failed to write M3U file")
		return fmt.Errorf("could not write the M3U export: %w", err)
	}

	if err := w.Flush(); err != nil {
		log.Error().Err(err).Msg("Failed to flush M3U file")
		return fmt.Errorf("could not finish writing the M3U export: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("could not close the M3U export: %w", err)
	}
	if err := os.Rename(tempFilePath, filePath); err != nil {
		log.Error().Err(err).Msg("Failed to publish M3U file")
		return fmt.Errorf("could not replace the M3U export: %w", err)
	}

	// Update cache with new content
	m.fileCacheMux.Lock()
	m.fileCache[template.Name] = strings.Split(b.String(), "\n")
	m.fileCacheMux.Unlock()
	return nil
}

// getFileContent returns the cached file content or reads from disk if not cached
func (m *M3uTools) getFileContent(templateName string) ([]string, error) {
	m.fileCacheMux.RLock()
	lines, found := m.fileCache[templateName]
	m.fileCacheMux.RUnlock()

	if found {
		return lines, nil
	}

	// Not in cache, read from disk
	filePath := internalM3UPath(templateName)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Store in cache
	lines = strings.Split(string(content), "\n")
	m.fileCacheMux.Lock()
	m.fileCache[templateName] = lines
	m.fileCacheMux.Unlock()

	return lines, nil
}

// writeFileContent writes the content back to the file
func (m *M3uTools) writeFileContent(templateName string, lines []string) error {
	filePath := internalM3UPath(templateName)

	// Use buffered writes
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, line := range lines {
		if _, err := w.WriteString(line + "\n"); err != nil {
			return err
		}
	}

	if err := w.Flush(); err != nil {
		return err
	}

	// Update cache
	m.fileCacheMux.Lock()
	m.fileCache[templateName] = lines
	m.fileCacheMux.Unlock()

	return nil
}

func (m *M3uTools) RenameTemplate(template *models.Template, oldTemplate string) {
	oldfile := internalM3UPath(oldTemplate)

	// Read content from cache or file
	lines, err := m.getFileContent(oldTemplate)
	if err != nil {
		log.Error().Msgf("Error reading file: %v", err)
		return
	}

	oldxmltvURL := fmt.Sprintf("http://%s:%d/xmltv/%s.xml", m.host, m.port, oldTemplate)
	xmltvURL := fmt.Sprintf("http://%s:%d/xmltv/%s.xml", m.host, m.port, template.Name)

	filter := fmt.Sprintf("#EXTM3U url-tvg=\"%s\" x-tvg-url=\"%s\"", oldxmltvURL, oldxmltvURL)
	newxml := fmt.Sprintf("#EXTM3U url-tvg=\"%s\" x-tvg-url=\"%s\"", xmltvURL, xmltvURL)

	// Process lines
	for i, line := range lines {
		if strings.Contains(line, filter) {
			lines[i] = strings.Replace(line, filter, newxml, 1)
		}
	}

	// Write to new file
	if err = m.writeFileContent(template.Name, lines); err != nil {
		log.Error().Msgf("Error writing file: %v", err)
		return
	}

	// Remove old file
	if err := os.Remove(oldfile); err != nil {
		log.Error().Msgf("Error removing file: %v", err)
		return
	}

	// Remove old file from cache
	m.fileCacheMux.Lock()
	delete(m.fileCache, oldTemplate)
	m.fileCacheMux.Unlock()

	go CreateEpgXML(*template)
}

func (m *M3uTools) RemoveTemplate(template *models.Template) {
	file := internalM3UPath(template.Name)
	if err := os.Remove(file); err != nil {
		log.Error().Msgf("Error removing file: %v", err)
		return
	}

	// Remove from cache
	m.fileCacheMux.Lock()
	delete(m.fileCache, template.Name)
	m.fileCacheMux.Unlock()
}

func (m *M3uTools) UpdateGroup(template *models.Template, group *models.TemplateGroup, oldGroup models.TemplateGroup) {
	// Get content from cache or file
	lines, err := m.getFileContent(template.Name)
	if err != nil {
		log.Error().Msgf("Error reading file: %v", err)
		return
	}

	filter := fmt.Sprintf("group-title=\"%s\"", oldGroup.Name)
	newGroup := fmt.Sprintf("group-title=\"%s\"", group.Name)

	// Process lines in-place
	modified := false
	for i, line := range lines {
		if strings.Contains(line, filter) {
			lines[i] = strings.Replace(line, filter, newGroup, 1)
			modified = true
		}
	}

	// Only write if we made changes
	if modified {
		if err = m.writeFileContent(template.Name, lines); err != nil {
			log.Error().Msgf("Error writing file: %v", err)
		}
	}
}

func (m *M3uTools) AddChannel(templateChannel *models.TemplateChannel, groupId int64) {
	//TODO ADD CHANNEL
}

func (m *M3uTools) UpdateChannel(template models.Template, channel *models.TemplateChannelLogo, oldChannel *models.TemplateChannel) {
	ctx := context.Background()
	// Get content from cache or file
	lines, err := m.getFileContent(template.Name)
	if err != nil {
		log.Error().Msgf("Error reading file: %v", err)
		return
	}

	logo, err := database.Db.GetLogo(ctx, oldChannel.LogoId)
	if err != nil {
		log.Warn().Msg(err.Error())
		return
	}
	oldLogoURL := fmt.Sprintf("http://%s:%d/%s", m.host, m.port, GetLogoUrl(logo.Name))
	logoURL := fmt.Sprintf("http://%s:%d/%s", m.host, m.port, channel.Logo)

	oldExportID := xmlTVChannelID(template, *oldChannel)
	newExportID := xmlTVChannelID(template, channel.TemplateChannel)
	filter := fmt.Sprintf("tvg-name=\"%s\" tvg-id=\"%s\" tvg-logo=\"%s\"", oldExportID, oldExportID, oldLogoURL)
	newChannel := fmt.Sprintf("tvg-name=\"%s\" tvg-id=\"%s\" tvg-logo=\"%s\"", newExportID, newExportID, logoURL)

	// Process lines in-place
	modified := false
	for i, line := range lines {
		if strings.Contains(line, filter) {
			lines[i] = strings.Replace(line, filter, newChannel, 1)
			modified = true
		}
	}

	// Only write if we made changes
	if modified {
		if err = m.writeFileContent(template.Name, lines); err != nil {
			log.Error().Msgf("Error writing file: %v", err)
			return
		}
	}

	go CreateEpgXML(template)
}

func (m *M3uTools) RemoveGroup(template *models.Template, group *models.TemplateGroup) {
	// Get content from cache or file
	lines, err := m.getFileContent(template.Name)
	if err != nil {
		log.Error().Msgf("Error reading file: %v", err)
		return
	}

	filter := fmt.Sprintf("group-title=\"%s\"", group.Name)

	// Find and remove lines in the group
	var newLines []string
	skipNextLine := false
	for _, line := range lines {
		if skipNextLine {
			skipNextLine = false
			continue
		} else if strings.Contains(line, filter) {
			skipNextLine = true
			continue
		}
		newLines = append(newLines, line)
	}

	// Write filtered content
	if err = m.writeFileContent(template.Name, newLines); err != nil {
		log.Error().Msgf("Error writing file: %v", err)
	}
}

func (m *M3uTools) RemoveChannel(template *models.Template, group *models.TemplateGroup, channel *models.TemplateChannel) {
	// Get content from cache or file
	lines, err := m.getFileContent(template.Name)
	if err != nil {
		log.Error().Msgf("Error reading file: %v", err)
		return
	}

	filterGroup := fmt.Sprintf("group-title=\"%s\"", group.Name)
	filterChannel := fmt.Sprintf("tvg-name=\"%s\"", channel.Name)

	// Find and remove channel lines
	var newLines []string
	skipNextLine := false
	for _, line := range lines {
		if skipNextLine {
			skipNextLine = false
			continue
		} else if strings.Contains(line, filterGroup) && strings.Contains(line, filterChannel) {
			skipNextLine = true
			continue
		}
		newLines = append(newLines, line)
	}

	// Write filtered content
	if err = m.writeFileContent(template.Name, newLines); err != nil {
		log.Error().Msgf("Error writing file: %v", err)
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
	ctx := context.Background()
	chNo := 1

	xmltvURL := fmt.Sprintf("http://%s:%d/xmltv/%s.xml", m.host, m.port, m.template.Name)

	_, err := writer.WriteString(fmt.Sprintf("#EXTM3U url-tvg=\"%s\" x-tvg-url=\"%s\"\n", xmltvURL, xmltvURL))
	if err != nil {
		log.Err(err)
		return err
	}

	groups, err := database.Db.GetTmplGroups(m.template.ID)
	if err != nil {
		log.Warn().Msg(err.Error())
		return err
	}

	for _, group := range groups {
		log.Info().Msgf("M3U Creation: Found Template Group: %s", group.Name)
		channels, err := database.Db.GetTmplChannelsByGroup(group.ID)
		if err != nil {
			return fmt.Errorf("could not load channels for group %q: %w", group.Name, err)
		}

		for _, channel := range channels {
			if items, err := database.Db.GetTmplChannelItemsByCh(channel.ID); err != nil {
				return fmt.Errorf("could not load sources for channel %q: %w", channel.Name, err)
			} else if items == nil || len(*items) == 0 {
				log.Debug().Str("channel", channel.Name).Msg("No channel URL found; omitting channel from M3U")
			} else {
				logo, err := database.Db.GetLogo(ctx, channel.LogoId)
				if err != nil {
					log.Warn().Msg(err.Error())
					continue
				}
				logoURL := fmt.Sprintf("http://%s:%d/%s", m.host, m.port, GetLogoUrl(logo.Name))
				channelURL := fmt.Sprintf("http://%s:%d/stream/%s", m.host, m.port, channel.Uuid)

				//log.Info().Msgf("M3U Creation: Adding Template Channel: %s", channel.Name)
				exportID := xmlTVChannelID(m.template, channel)
				if _, err = writer.WriteString(fmt.Sprintf("#EXTINF:-1 tvg-chno=\"%d\" tvg-name=\"%s\" tvg-id=\"%s\" tvg-logo=\"%s\" group-title=\"%s\",%s\n%s\n", chNo, exportID, exportID, logoURL, group.Name, channel.Name, channelURL)); err != nil {
					return fmt.Errorf("could not encode channel %q: %w", channel.Name, err)
				}
				chNo++
			}
		}
	}

	if chNo == 1 {
		return fmt.Errorf("the lineup has no playable channels with source variants")
	}
	return writer.Flush()
}
