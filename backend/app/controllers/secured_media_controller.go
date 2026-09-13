package controllers

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"xivi/backend/pkg/middleware"
	"xivi/backend/pkg/security"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

func securedQueryToken(c *fiber.Ctx) (string, bool) {
	credential, ok := middleware.CurrentMediaCredential(c)
	return credential.Token, ok && credential.Token != ""
}

func sanitizeM3UValue(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' || r == ',' {
			return -1
		}
		return r
	}, value)
	return strings.TrimSpace(value)
}

func mediaURL(base, path, token string) string {
	parsed, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil {
		return ""
	}
	query := parsed.Query()
	query.Set("access_token", token)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func setPrivateValidator(c *fiber.Ctx, contentIdentity []byte) bool {
	digest := sha256.Sum256(contentIdentity)
	etag := `"` + hex.EncodeToString(digest[:]) + `"`
	c.Set(fiber.HeaderETag, etag)
	c.Set(fiber.HeaderCacheControl, "private, max-age=0, must-revalidate")
	for _, candidate := range strings.Split(c.Get(fiber.HeaderIfNoneMatch), ",") {
		if strings.TrimSpace(candidate) == etag || strings.TrimSpace(candidate) == "*" {
			c.Status(fiber.StatusNotModified)
			return true
		}
	}
	return false
}

func GetDynamicM3U(c *fiber.Ctx) error {
	lineupID, ok := security.ParsePositiveID(c.Params("lineup_id"))
	if !ok {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	return getDynamicM3U(c, lineupID)
}

func GetShortM3U(c *fiber.Ctx) error {
	credential, ok := middleware.CurrentMediaCredential(c)
	if !ok || credential.LineupID <= 0 {
		return v2Error(c, fiber.StatusUnauthorized, "media_authentication_required", "A valid media output code is required.", false)
	}
	return getDynamicM3U(c, credential.LineupID)
}

func getDynamicM3U(c *fiber.Ctx, lineupID int64) error {
	token, ok := securedQueryToken(c)
	if !ok {
		return v2Error(c, fiber.StatusUnauthorized, "media_authentication_required", "A media key is required.", false)
	}
	lineup, err := database.Db.GetTemplate(lineupID)
	if err != nil {
		return v2Error(c, fiber.StatusNotFound, "lineup_not_found", "The lineup was not found.", false)
	}
	base := security.MediaBaseURLForRequest(c)
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)
	xmltv := mediaURL(base, "/media/v1/lineups/"+strconv.FormatInt(lineupID, 10)+"/guide.xml", token)
	if credential, aliasRequest := middleware.CurrentMediaCredential(c); aliasRequest && credential.OutputCode != "" {
		xmltv = strings.TrimRight(base, "/") + "/x/" + url.PathEscape(credential.OutputCode)
	}
	_, _ = fmt.Fprintf(writer, "#EXTM3U url-tvg=\"%s\" x-tvg-url=\"%s\"\n", xmltv, xmltv)
	groups, err := database.Db.GetTmplGroups(lineupID)
	if err != nil {
		return v2Error(c, 500, "playlist_failed", "The playlist could not be generated.", true)
	}
	channelNumber := 1
	for _, group := range groups {
		channels, err := database.Db.GetTmplChannelsByGroup(group.ID)
		if err != nil {
			return v2Error(c, 500, "playlist_failed", "The playlist could not be generated.", true)
		}
		for _, channel := range channels {
			items, err := database.Db.GetTmplChannelItemsByCh(channel.ID)
			if err != nil || items == nil || len(*items) == 0 {
				continue
			}
			logo, _ := database.Db.GetLogo(c.UserContext(), channel.LogoId)
			logoURL := ""
			if logo != nil && logo.Name != "" {
				logoURL = mediaURL(base, "/images/"+url.PathEscape(logo.Name)+".png?lineup_id="+strconv.FormatInt(lineupID, 10), token)
			}
			exportID := sanitizeM3UValue(channel.Name)
			if channel.TvgID != nil && strings.TrimSpace(*channel.TvgID) != "" {
				exportID = sanitizeM3UValue(*channel.TvgID)
			}
			stream := mediaURL(base, "/stream/"+url.PathEscape(channel.Uuid), token)
			_, _ = fmt.Fprintf(writer, "#EXTINF:-1 tvg-chno=\"%d\" tvg-name=\"%s\" tvg-id=\"%s\" tvg-logo=\"%s\" group-title=\"%s\",%s\n%s\n",
				channelNumber, exportID, exportID, logoURL, sanitizeM3UValue(group.Name), sanitizeM3UValue(channel.Name), stream)
			channelNumber++
		}
	}
	if channelNumber == 1 {
		return v2Error(c, 409, "empty_lineup", "The lineup has no playable channels.", false)
	}
	_ = writer.Flush()
	c.Set(fiber.HeaderContentType, "audio/x-mpegurl; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"xivi-lineup-%d.m3u\"", lineupID))
	if setPrivateValidator(c, buffer.Bytes()) {
		return nil
	}
	_ = lineup
	return c.Send(buffer.Bytes())
}

func GetDynamicXMLTV(c *fiber.Ctx) error {
	lineupID, ok := security.ParsePositiveID(c.Params("lineup_id"))
	if !ok {
		return v2Error(c, 400, "invalid_lineup", "The lineup id is invalid.", false)
	}
	return getDynamicXMLTV(c, lineupID)
}

func GetShortXMLTV(c *fiber.Ctx) error {
	credential, ok := middleware.CurrentMediaCredential(c)
	if !ok || credential.LineupID <= 0 {
		return v2Error(c, fiber.StatusUnauthorized, "media_authentication_required", "A valid media output code is required.", false)
	}
	return getDynamicXMLTV(c, credential.LineupID)
}

func getDynamicXMLTV(c *fiber.Ctx, lineupID int64) error {
	token, credentialOK := securedQueryToken(c)
	if !credentialOK {
		return v2Error(c, fiber.StatusUnauthorized, "media_authentication_required", "A media key is required.", false)
	}
	path := utils.LineupXMLTVPath(lineupID)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return v2Error(c, 404, "guide_not_published", "The lineup guide has not been published yet.", false)
	}
	c.Set(fiber.HeaderContentType, "application/xml; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"xivi-lineup-%d.xml\"", lineupID))
	base := security.MediaBaseURLForRequest(c)
	identity := []byte(fmt.Sprintf("%d|%d|%d|%s|%s", lineupID, info.Size(), info.ModTime().UnixNano(), base, token))
	c.Set(fiber.HeaderLastModified, info.ModTime().UTC().Format(http.TimeFormat))
	if setPrivateValidator(c, identity) {
		return nil
	}
	c.Context().Response.SetBodyStreamWriter(func(writer *bufio.Writer) {
		file, openErr := os.Open(path)
		if openErr != nil {
			return
		}
		defer file.Close()
		_ = rewriteSecuredXMLTV(file, writer, base, token, lineupID)
	})
	return nil
}

func rewriteSecuredXMLTV(reader io.Reader, writer io.Writer, base, token string, lineupID int64) error {
	decoder := xml.NewDecoder(reader)
	encoder := xml.NewEncoder(writer)
	for {
		xmlToken, err := decoder.Token()
		if err == io.EOF {
			return encoder.Flush()
		}
		if err != nil {
			return err
		}
		if start, ok := xmlToken.(xml.StartElement); ok && start.Name.Local == "icon" {
			attributes := start.Attr[:0]
			for _, attribute := range start.Attr {
				if attribute.Name.Local != "src" {
					attributes = append(attributes, attribute)
					continue
				}
				parsed, parseErr := url.Parse(attribute.Value)
				if parseErr != nil || !strings.HasPrefix("/"+strings.TrimLeft(parsed.Path, "/"), "/images/") {
					// Upstream icon URLs are deliberately omitted: they can expose
					// provider credentials or bypass Xivi's revocation boundary.
					continue
				}
				attribute.Value = mediaURL(base, "/"+strings.TrimLeft(parsed.Path, "/")+"?lineup_id="+strconv.FormatInt(lineupID, 10), token)
				attributes = append(attributes, attribute)
			}
			start.Attr = attributes
			xmlToken = start
		}
		if err := encoder.EncodeToken(xmlToken); err != nil {
			return err
		}
	}
}

func GetSecuredImage(c *fiber.Ctx) error {
	asset := c.Params("asset")
	if filepath.Base(asset) != asset || !strings.HasSuffix(strings.ToLower(asset), ".png") {
		return c.SendStatus(fiber.StatusNotFound)
	}
	root, err := filepath.Abs(settings.LOGO_FILEPATH)
	if err != nil {
		return c.SendStatus(500)
	}
	path, err := filepath.Abs(filepath.Join(root, asset))
	if err != nil {
		return c.SendStatus(404)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return c.SendStatus(404)
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return c.SendStatus(404)
	}
	c.Set(fiber.HeaderContentType, "image/png")
	// Revalidate authorization on every request so revoking a key or lineup grant
	// cannot leave a usable protected logo in a shared browser cache.
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	return security.SendFileLiteral(c, path)
}
