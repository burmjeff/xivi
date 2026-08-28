package controllers

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/middleware"
	"xivi/backend/pkg/outbound"
	"xivi/backend/pkg/security"
	"xivi/backend/pkg/streaming"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

const streamOperationTimeout = 30 * time.Second

func streamSources(channels []models.ChannelUrl) []streaming.Source {
	sources := make([]streaming.Source, 0, len(channels))
	for _, channel := range channels {
		sources = append(sources, streaming.Source{URL: channel.Url, PoolID: channel.PlaylistID,
			PoolName: channel.PlaylistName, ConnectionLimit: channel.ConnectionLimit})
	}
	return sources
}

func acquireStreamSession(ctx context.Context, streamID, playbackID, protocol string) (*streaming.Session, []models.ChannelUrl, string, error) {
	if !settings.APP_SETTINGS.Streaming.Proxy {
		return nil, nil, "", errors.New("secure playback requires stream proxying")
	}
	channels, err := database.Db.GetChannelsbyUuid(ctx, streamID)
	if err != nil {
		return nil, nil, "", fmt.Errorf("load ordered source variants: %w", err)
	}
	if channels == nil || len(*channels) == 0 {
		return nil, nil, "", fmt.Errorf("no streamable source variants are available")
	}
	if _, active := streaming.DefaultManager.Get(streamID); !active {
		validated := make([]models.ChannelUrl, 0, len(*channels))
		for _, channel := range *channels {
			if _, validationErr := outbound.Validate(ctx, channel.Url, true); validationErr == nil {
				validated = append(validated, channel)
			}
		}
		if len(validated) == 0 {
			return nil, nil, "", fmt.Errorf("all source variants were rejected by the outbound network policy")
		}
		channels = &validated
	}
	if playbackID != "" {
		var session *streaming.Session
		var handle streaming.PlaybackHandle
		var acquireErr error
		if protocol == "mpegts" {
			session, handle, acquireErr = streaming.DefaultManager.StartPlaybackSources(ctx, playbackID, streamID, protocol, streamSources(*channels))
		} else {
			session, handle, acquireErr = streaming.DefaultManager.AcquirePlaybackSources(ctx, playbackID, streamID, protocol, streamSources(*channels))
		}
		return session, *channels, handle.ClientID, acquireErr
	}
	var session *streaming.Session
	if protocol == "mpegts" {
		session, err = streaming.DefaultManager.StartSources(ctx, streamID, streamSources(*channels))
	} else {
		session, err = streaming.DefaultManager.AcquireSources(ctx, streamID, streamSources(*channels))
	}
	if err != nil {
		return nil, *channels, "", err
	}
	return session, *channels, "", nil
}

func streamError(c *fiber.Ctx, status int, message string, err error) error {
	var incidentID string
	if session, ok := streaming.DefaultManager.Get(c.Params("stream_id")); ok {
		incidentID = session.IncidentID()
	}
	return streamErrorForSession(c, status, message, err, incidentID)
}

func streamErrorForSession(c *fiber.Ctx, status int, message string, err error, incidentID string) error {
	detail := ""
	code, retryable := streaming.ClassifyError(err)
	if err != nil {
		detail = streaming.SanitizeDiagnostic(err.Error())
	}
	log.Error().Str("error", detail).Str("stream_id", c.Params("stream_id")).Msg(message)
	if incidentID != "" {
		c.Set("X-Xivi-Incident-ID", incidentID)
	}
	return c.Status(status).JSON(fiber.Map{
		"error": true, "code": code, "message": message, "msg": message,
		"detail": detail, "incident_id": incidentID, "retryable": retryable,
	})
}

func streamClientMetadata(c *fiber.Ctx, protocol, id string) streaming.ClientMetadata {
	metadata := streaming.ClientMetadata{ID: strings.Clone(id), Protocol: protocol, RemoteIP: security.RequestNetworkInfo(c).IP.String(),
		Method: c.Method() + " " + c.Path(), UserAgent: strings.Clone(c.Get(fiber.HeaderUserAgent))}
	if principal, ok := middleware.Principal(c); ok {
		if session, sessionOK := middleware.CurrentSession(c); sessionOK {
			metadata.AuthKind, metadata.AuthID, metadata.OwnerUserID = "session", session.ID, principal.UserID
		} else {
			metadata.AuthKind, metadata.AuthID, metadata.OwnerUserID = "user", principal.UserID, principal.UserID
		}
	} else if credential, ok := middleware.CurrentMediaCredential(c); ok {
		if credential.Key != nil {
			metadata.AuthKind, metadata.AuthID, metadata.OwnerUserID = "media_key", credential.Key.ID, credential.Key.UserID
		} else {
			metadata.AuthKind, metadata.AuthID = "virtual_tuner", credential.VirtualLineupID
		}
	}
	return metadata
}

type streamAuthorization struct {
	kind    string
	id      int64
	role    string
	version int64
}

func captureStreamAuthorization(c *fiber.Ctx) streamAuthorization {
	if principal, ok := middleware.Principal(c); ok {
		if session, sessionOK := middleware.CurrentSession(c); sessionOK {
			return streamAuthorization{kind: "session", id: session.ID}
		}
		return streamAuthorization{kind: "user", id: principal.UserID, role: principal.Role}
	}
	if credential, ok := middleware.CurrentMediaCredential(c); ok {
		if credential.Key != nil {
			return streamAuthorization{kind: "media_key", id: credential.Key.ID}
		}
		_, version, _ := security.ValidateVirtualTunerToken(credential.Token)
		return streamAuthorization{kind: "virtual_tuner", id: credential.VirtualLineupID, version: version}
	}
	return streamAuthorization{}
}

func (authorization streamAuthorization) allows(ctx context.Context, streamID string) bool {
	var allowed bool
	var err error
	switch authorization.kind {
	case "session":
		allowed, err = database.Db.SessionCanAccessChannel(ctx, authorization.id, streamID)
	case "user":
		allowed, err = database.Db.UserCanAccessChannel(ctx, authorization.id, authorization.role, streamID)
	case "media_key":
		allowed, err = database.Db.MediaKeyAllowsChannel(ctx, authorization.id, streamID)
	case "virtual_tuner":
		allowed, err = database.Db.VirtualTunerAllowsChannel(ctx, authorization.id, authorization.version, streamID)
	default:
		return false
	}
	return err == nil && allowed
}

func normalizedPlaybackID(namespace, candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || len(candidate) > 512 {
		return ""
	}
	sum := sha256.Sum256([]byte(namespace + "|" + candidate))
	return "ply_" + hex.EncodeToString(sum[:12])
}

// requestPlaybackID accepts only explicit player or playback-session IDs.
// Device/server IDs and IP addresses are intentionally excluded because one
// Plex/Jellyfin server or NAT address can represent multiple simultaneous
// viewers.
func requestPlaybackID(c *fiber.Ctx) string {
	if token := c.Query("playback_token"); len(token) == 28 && strings.HasPrefix(token, "ply_") {
		if _, err := hex.DecodeString(strings.TrimPrefix(token, "ply_")); err == nil {
			return strings.Clone(token)
		}
	}
	for _, key := range []string{"playback_id", "viewer_id"} {
		if candidate := c.Query(key); candidate != "" && !strings.HasPrefix(candidate, "hls_") {
			return normalizedPlaybackID("viewer", candidate)
		}
	}
	for _, key := range []string{"PlaySessionId", "playSessionId", "session_id"} {
		if candidate := c.Query(key); candidate != "" {
			return normalizedPlaybackID("media-session", candidate)
		}
	}
	for _, key := range []string{
		"X-Xivi-Playback-ID",
		"X-Plex-Session-Identifier",
		"X-Plex-Playback-Session-Id",
		"X-Emby-Session-Id",
		"X-Jellyfin-Session-Id",
		"X-MediaBrowser-Session-Id",
		"X-Playback-Session-Id",
	} {
		if candidate := c.Get(key); candidate != "" {
			return normalizedPlaybackID("media-session", candidate)
		}
	}
	return ""
}

func hlsViewerID(c *fiber.Ctx, streamID string) string {
	if candidate := c.Query("viewer_id"); len(candidate) > 0 && len(candidate) <= 128 {
		valid := true
		for _, character := range candidate {
			if !(character == '-' || character == '_' || character >= '0' && character <= '9' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z') {
				valid = false
				break
			}
		}
		if valid {
			return strings.Clone(candidate)
		}
	}
	sum := sha256.Sum256([]byte(streamID + "|" + c.IP() + "|" + c.Get(fiber.HeaderUserAgent)))
	return "hls_" + hex.EncodeToString(sum[:8])
}

// GetStream returns the shared MPEG-TS transport stream for a channel UUID.
// Every viewer receives data from a bounded Go fan-out queue, so a slow or
// disconnected client cannot stall the one upstream GStreamer producer.
// @Router /stream/{stream_id} [get]
func GetStream(c *fiber.Ctx) error {
	streamID := strings.Clone(c.Params("stream_id"))
	if streamID == "" {
		return streamError(c, fiber.StatusBadRequest, "A stream id is required.", nil)
	}
	if !settings.APP_SETTINGS.Streaming.Proxy {
		return streamErrorForSession(c, fiber.StatusServiceUnavailable,
			"Secure playback requires stream proxying to be enabled.", errors.New("stream proxy is disabled"), "")
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), streamOperationTimeout)
	defer cancel()
	playbackID := requestPlaybackID(c)
	session, _, trackedClientID, err := acquireStreamSession(ctx, streamID, playbackID, "mpegts")
	if err != nil {
		incidentID := ""
		if session != nil {
			incidentID = session.IncidentID()
		}
		if errors.Is(err, streaming.ErrSessionStopping) {
			c.Set(fiber.HeaderRetryAfter, "1")
			return streamErrorForSession(c, fiber.StatusServiceUnavailable,
				"The previous stream is still stopping; retry shortly.", err, incidentID)
		}
		return streamErrorForSession(c, fiber.StatusBadGateway, "The stream could not start.", err, incidentID)
	}
	subscription := session.Subscribe()
	clientID, allowed := session.RegisterClient(streamClientMetadata(c, "mpegts", trackedClientID), subscription.Close)
	if !allowed {
		subscription.Close()
		return c.SendStatus(fiber.StatusGone)
	}
	c.Set("X-Xivi-Incident-ID", session.IncidentID())
	c.Set("X-Xivi-Connection-ID", clientID)
	c.Set(fiber.HeaderContentType, "video/MP2T")
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderConnection, "keep-alive")
	authorization := captureStreamAuthorization(c)
	c.Context().Response.SetBodyStreamWriter(func(writer *bufio.Writer) {
		defer subscription.Close()
		reason := "client_disconnected"
		defer func() { session.CloseClient(clientID, reason) }()
		authorizationDone := make(chan struct{})
		defer close(authorizationDone)
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-authorizationDone:
					return
				case <-ticker.C:
					authorizationContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					stillAllowed := authorization.allows(authorizationContext, streamID)
					cancel()
					if !stillAllowed {
						session.ReleaseClient(clientID, "authorization_revoked")
						return
					}
				}
			}
		}()
		buffered := 0
		firstChunk := true
		lastFlush := time.Now()
		for {
			chunk, ok := subscription.Next()
			if !ok {
				if subscription.CloseReason() == "slow_client_dropped" {
					reason = "slow_client_dropped"
				}
				_ = writer.Flush()
				return
			}
			if _, err := writer.Write(chunk); err != nil {
				reason = "client_write_failed"
				return
			}
			session.AddClientBytes(clientID, len(chunk))
			buffered += len(chunk)
			if firstChunk || buffered >= 256*1024 || time.Since(lastFlush) >= 200*time.Millisecond {
				if err := writer.Flush(); err != nil {
					reason = "client_flush_failed"
					return
				}
				buffered = 0
				firstChunk = false
				lastFlush = time.Now()
			}
		}
	})
	return nil
}

// GetHlsStream starts or reuses the same producer as GetStream and returns its
// validated live playlist. Readiness requires a non-empty segment, not merely
// the existence of a playlist file.
// @Router /stream/hls/{stream_id} [get]
func GetHlsStream(c *fiber.Ctx) error {
	streamID := strings.Clone(c.Params("stream_id"))
	if streamID == "" {
		return streamError(c, fiber.StatusBadRequest, "A stream id is required.", nil)
	}
	if !settings.APP_SETTINGS.Streaming.Proxy {
		return streamErrorForSession(c, fiber.StatusServiceUnavailable,
			"Secure playback requires stream proxying to be enabled.", errors.New("stream proxy is disabled"), "")
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), streamOperationTimeout)
	defer cancel()
	playbackID := requestPlaybackID(c)
	session, _, trackedClientID, err := acquireStreamSession(ctx, streamID, playbackID, "hls")
	if err != nil {
		incidentID := ""
		if session != nil {
			incidentID = session.IncidentID()
		}
		if errors.Is(err, streaming.ErrSessionStopping) {
			c.Set(fiber.HeaderRetryAfter, "1")
			return streamErrorForSession(c, fiber.StatusServiceUnavailable,
				"The previous stream is still stopping; retry shortly.", err, incidentID)
		}
		return streamErrorForSession(c, fiber.StatusBadGateway, "The stream could not start.", err, incidentID)
	}
	viewerID := trackedClientID
	if viewerID == "" {
		viewerID = hlsViewerID(c, streamID)
	}
	if _, allowed := session.RegisterClient(streamClientMetadata(c, "hls", viewerID), nil); !allowed {
		return c.SendStatus(fiber.StatusGone)
	}
	c.Set("X-Xivi-Incident-ID", session.IncidentID())
	c.Set("X-Xivi-Connection-ID", viewerID)
	c.Locals("stream_client_id", viewerID)
	c.Locals("stream_playback_id", playbackID)
	if err := session.WaitHLS(ctx); err != nil {
		diagnostic := fmt.Errorf("HLS playlist did not become ready: %w", err)
		session.RecordClientEvent(viewerID, "error", "hls_not_ready", diagnostic.Error(), "")
		session.CloseClient(viewerID, "hls_start_failed")
		return streamErrorForSession(c, fiber.StatusGatewayTimeout, "The HLS playlist did not become ready.", diagnostic, session.IncidentID())
	}
	return sendHLSFile(c, streamID, "playlist.m3u8")
}

// GetHlsAsset serves only files belonging to a live managed session. This both
// blocks path traversal/stale-file access and lets playlist/segment requests
// keep HLS-only sessions alive until the viewer stops polling.
func GetHlsAsset(c *fiber.Ctx) error {
	streamID := c.Params("stream_id")
	asset := c.Params("asset")
	if filepath.Base(asset) != asset || (!strings.HasSuffix(asset, ".ts") && asset != "playlist.m3u8") {
		return c.SendStatus(fiber.StatusNotFound)
	}
	session, ok := streaming.DefaultManager.Get(streamID)
	if !ok || !streaming.DefaultManager.Touch(streamID) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	playbackID := requestPlaybackID(c)
	viewerID := ""
	if playbackID != "" {
		handle, current := streaming.DefaultManager.ResolvePlayback(playbackID, streamID)
		if !current {
			return c.SendStatus(fiber.StatusGone)
		}
		viewerID = handle.ClientID
	} else {
		viewerID = hlsViewerID(c, streamID)
	}
	if _, allowed := session.RegisterClient(streamClientMetadata(c, "hls", viewerID), nil); !allowed {
		return c.SendStatus(fiber.StatusGone)
	}
	c.Set("X-Xivi-Incident-ID", session.IncidentID())
	c.Set("X-Xivi-Connection-ID", viewerID)
	c.Locals("stream_client_id", viewerID)
	c.Locals("stream_playback_id", playbackID)
	return sendHLSFile(c, streamID, asset)
}

// scopeHLSPlaylist makes every segment URI independent of the URL used to
// request the manifest. The public entry point intentionally omits a trailing
// slash (/stream/hls/:id), so a bare segment name would otherwise resolve to
// /stream/hls/segment.ts and lose the stream id entirely.
func scopeHLSPlaylist(content []byte, streamID, identityKey, identityValue string) []byte {
	lines := strings.Split(string(content), "\n")
	segmentRoot := "/stream/hls/" + url.PathEscape(streamID) + "/"
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parsed, err := url.Parse(trimmed)
		if err != nil {
			continue
		}
		segment := filepath.Base(parsed.Path)
		if segment == "." || segment == "" {
			continue
		}
		// HLS media is always served by this managed session. Discard any
		// origin inherited from the producer playlist and emit one canonical,
		// root-relative asset URL for browsers and external HLS clients.
		parsed.Scheme = ""
		parsed.Host = ""
		parsed.User = nil
		parsed.Path = segmentRoot + url.PathEscape(segment)
		parsed.RawPath = ""
		parsed.Fragment = ""
		if identityKey != "" && identityValue != "" {
			query := parsed.Query()
			query.Set(identityKey, identityValue)
			parsed.RawQuery = query.Encode()
		}
		lines[index] = parsed.String()
	}
	return []byte(strings.Join(lines, "\n"))
}

func sendHLSFile(c *fiber.Ctx, streamID, asset string) error {
	path := filepath.Join(settings.STREAM_FILEPATH, streamID, asset)
	if asset == "playlist.m3u8" {
		var snapshot *streaming.HLSPlaylistSnapshot
		var err error
		for attempt := 0; attempt < 4; attempt++ {
			session, active := streaming.DefaultManager.Get(streamID)
			if active {
				snapshot, err = session.HLSPlaylistSnapshot()
			} else {
				// The media routes require a managed session; this fallback keeps
				// the stable-file helper independently testable.
				snapshot, err = streaming.ReadHLSPlaylist(path)
			}
			if err == nil {
				identityKey, identityValue := "", ""
				if token := c.Query("access_token"); token != "" {
					identityKey, identityValue = "access_token", token
				} else if playbackID, ok := c.Locals("stream_playback_id").(string); ok && playbackID != "" {
					identityKey, identityValue = "playback_token", playbackID
				} else if token := c.Query("playback_token"); token != "" {
					identityKey, identityValue = "playback_token", token
				} else if _, active := streaming.DefaultManager.Get(streamID); active || c.Query("viewer_id") != "" {
					identityKey, identityValue = "viewer_id", hlsViewerID(c, streamID)
				}
				content := scopeHLSPlaylist(snapshot.Content, streamID, identityKey, identityValue)
				c.Set(fiber.HeaderContentType, "application/vnd.apple.mpegurl")
				c.Set(fiber.HeaderCacheControl, "no-cache, no-store, must-revalidate")
				c.Set("Pragma", "no-cache")
				c.Set("Expires", "0")
				if session, ok := streaming.DefaultManager.Get(streamID); ok {
					clientID, _ := c.Locals("stream_client_id").(string)
					if clientID == "" {
						clientID = hlsViewerID(c, streamID)
					}
					session.AddClientBytes(clientID, len(content))
				}
				return c.Send(content)
			}
			time.Sleep(15 * time.Millisecond)
		}
		log.Debug().Str("error", streaming.SanitizeDiagnostic(err.Error())).Str("stream_id", streamID).Msg("HLS playlist rewrite was not ready to serve")
		c.Set("Retry-After", "1")
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return c.SendStatus(fiber.StatusNotFound)
	}
	c.Set(fiber.HeaderContentType, "video/MP2T")
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	if session, ok := streaming.DefaultManager.Get(streamID); ok {
		clientID, _ := c.Locals("stream_client_id").(string)
		if clientID == "" {
			clientID = hlsViewerID(c, streamID)
		}
		session.AddClientBytes(clientID, int(info.Size()))
	}
	return c.SendFile(path)
}

// V2StreamingStatus exposes enough state to diagnose startup, failover, stalls
// and slow-client eviction without relying on server logs.
func V2StreamingStatus(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"summary":            streaming.DefaultManager.Summary(),
		"sessions":           streaming.DefaultManager.Snapshots(),
		"source_connections": streaming.DefaultManager.ConnectionUsage(),
	})
}

type studioStreamItem struct {
	streaming.SessionSnapshot
	ChannelID      int64                `json:"channel_id,omitempty"`
	ChannelName    string               `json:"channel_name"`
	LogoURL        string               `json:"logo_url,omitempty"`
	ProgrammeTitle string               `json:"programme_title,omitempty"`
	ProgrammeStart string               `json:"programme_start,omitempty"`
	ProgrammeEnd   string               `json:"programme_end,omitempty"`
	NextTitle      string               `json:"next_title,omitempty"`
	SourceName     string               `json:"source_name,omitempty"`
	SourceGroup    string               `json:"source_group,omitempty"`
	SourcePlaylist string               `json:"source_playlist,omitempty"`
	Events         []models.StreamEvent `json:"events"`
}

func enrichStreamSnapshot(ctx context.Context, snapshot streaming.SessionSnapshot) studioStreamItem {
	item := studioStreamItem{SessionSnapshot: snapshot, ChannelName: snapshot.ID, Events: []models.StreamEvent{}}
	if events, err := database.Db.ListStreamEvents(ctx, snapshot.IncidentID, 100); err == nil {
		item.Events = events
	}
	if identity, err := database.Db.GetStreamIdentity(ctx, snapshot.ID); err == nil {
		item.ChannelID, item.ChannelName = identity.ID, identity.Name
		if identity.LogoName != "" {
			item.LogoURL = utils.GetLogoUrl(identity.LogoName)
		}
		if identity.TvgID != nil {
			if programme, programmeErr := database.Db.GetCurrentProgramme(ctx, *identity.TvgID, time.Now()); programmeErr == nil {
				item.ProgrammeTitle = programme.Title.Value
				item.ProgrammeStart, item.ProgrammeEnd = programme.Start.Time.Format(time.RFC3339), programme.Stop.Time.Format(time.RFC3339)
				if next, nextErr := database.Db.GetCurrentProgramme(ctx, *identity.TvgID, programme.Stop.Time); nextErr == nil {
					item.NextTitle = next.Title.Value
				}
			}
		}
	}
	if sources, err := database.Db.GetChannelsbyUuid(ctx, snapshot.ID); err == nil && sources != nil && snapshot.SourcePosition > 0 && snapshot.SourcePosition <= len(*sources) {
		source := (*sources)[snapshot.SourcePosition-1]
		item.SourceName, item.SourceGroup, item.SourcePlaylist = source.SourceName, source.GroupName, source.PlaylistName
	}
	return item
}

func V2StudioStreams(c *fiber.Ctx) error {
	ctx := c.UserContext()
	snapshots := streaming.DefaultManager.Snapshots()
	items := make([]studioStreamItem, 0, len(snapshots))
	for _, snapshot := range snapshots {
		items = append(items, enrichStreamSnapshot(ctx, snapshot))
	}
	history, err := database.Db.ListStreamSessions(ctx, 50)
	if err != nil {
		log.Error().Err(err).Msg("Stream history could not be loaded")
		return v2Error(c, fiber.StatusInternalServerError, "stream_history_failed", "Stream history could not be loaded.", true)
	}
	events, err := database.Db.ListStreamEvents(ctx, "", 50)
	if err != nil {
		log.Error().Err(err).Msg("Stream events could not be loaded")
		return v2Error(c, fiber.StatusInternalServerError, "stream_events_failed", "Stream events could not be loaded.", true)
	}
	return c.JSON(fiber.Map{"summary": streaming.DefaultManager.Summary(), "items": items,
		"history": history, "recent_events": events, "proxy_enabled": settings.APP_SETTINGS.Streaming.Proxy,
		"source_connections": streaming.DefaultManager.ConnectionUsage()})
}

func V2StudioStreamHistory(c *fiber.Ctx) error {
	incidentID := c.Params("incident_id")
	connections, err := database.Db.ListStreamConnections(c.UserContext(), incidentID)
	if err != nil {
		log.Error().Err(err).Msg("Stream connections could not be loaded")
		return v2Error(c, fiber.StatusInternalServerError, "stream_connections_failed", "Stream connections could not be loaded.", true)
	}
	events, err := database.Db.ListStreamEvents(c.UserContext(), incidentID, 250)
	if err != nil {
		log.Error().Err(err).Msg("Stream events could not be loaded")
		return v2Error(c, fiber.StatusInternalServerError, "stream_events_failed", "Stream events could not be loaded.", true)
	}
	return c.JSON(fiber.Map{"connections": connections, "events": events})
}

func V2StopStream(c *fiber.Ctx) error {
	if !streaming.DefaultManager.StopManual(c.Params("stream_id")) {
		return v2Error(c, fiber.StatusNotFound, "stream_not_found", "That stream is no longer active.", false)
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"status": "stop_requested"})
}

func V2FailoverStream(c *fiber.Ctx) error {
	if !streaming.DefaultManager.ForceNextSource(c.Params("stream_id")) {
		return v2Error(c, fiber.StatusConflict, "failover_unavailable", "The source could not be advanced right now.", true)
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"status": "failover_requested"})
}

func V2RestartStream(c *fiber.Ctx) error {
	if !streaming.DefaultManager.RestartSource(c.Params("stream_id")) {
		return v2Error(c, fiber.StatusConflict, "restart_unavailable", "The active source could not be restarted right now.", true)
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"status": "restart_requested"})
}

func V2PrewarmStream(c *fiber.Ctx) error {
	if settings.APP_SETTINGS.Streaming.PrewarmChannels <= 0 {
		return c.SendStatus(fiber.StatusNoContent)
	}
	streamID := strings.Clone(c.Params("stream_id"))
	channels, err := database.Db.GetChannelsbyUuid(c.UserContext(), streamID)
	if err != nil || channels == nil || len(*channels) == 0 {
		return v2Error(c, fiber.StatusNotFound, "stream_not_found", "That channel has no playable sources.", false)
	}
	sources := streamSources(*channels)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if _, err := streaming.DefaultManager.PrewarmSources(ctx, streamID, sources); err != nil {
			log.Debug().Str("error", streaming.SanitizeDiagnostic(err.Error())).Str("stream_id", streamID).Msg("Optional stream prewarm was skipped")
		}
	}()
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"status": "prewarm_requested"})
}

func V2DisconnectStreamClient(c *fiber.Ctx) error {
	if !streaming.DefaultManager.DisconnectClient(c.Params("stream_id"), c.Params("connection_id")) {
		return v2Error(c, fiber.StatusNotFound, "connection_not_found", "That viewer is no longer connected.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type releasePlaybackRequest struct {
	PlaybackID string `json:"playback_id"`
	StreamID   string `json:"stream_id"`
}

// V2ReleasePlayback lets controlled players surrender a source connection
// immediately. It is deliberately idempotent so unload beacons can race a
// channel switch without turning normal navigation into an error.
func V2ReleasePlayback(c *fiber.Ctx) error {
	request := releasePlaybackRequest{}
	if err := decodeStrict(c, &request); err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_request", "Playback release data is invalid.", false)
	}
	playbackID := normalizedPlaybackID("viewer", request.PlaybackID)
	if playbackID == "" || request.StreamID == "" {
		return v2Error(c, fiber.StatusBadRequest, "missing_playback_identity", "A playback id and stream id are required.", false)
	}
	principal, _ := middleware.Principal(c)
	allowed, err := database.Db.UserCanAccessChannel(c.UserContext(), principal.UserID, principal.Role, request.StreamID)
	if err != nil || !allowed {
		return v2Error(c, fiber.StatusNotFound, "stream_not_found", "That stream was not found.", false)
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), streamOperationTimeout)
	defer cancel()
	streaming.DefaultManager.ReleasePlayback(ctx, playbackID, strings.Clone(request.StreamID))
	return c.SendStatus(fiber.StatusNoContent)
}

func V2StreamTelemetry(c *fiber.Ctx) error {
	var request struct {
		StreamID string         `json:"stream_id"`
		ViewerID string         `json:"viewer_id"`
		Severity string         `json:"severity"`
		Code     string         `json:"code"`
		Message  string         `json:"message"`
		Details  map[string]any `json:"details"`
	}
	if err := decodeStrict(c, &request); err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_telemetry", "Player telemetry was not valid.", false)
	}
	if !safeTelemetryIdentifier(request.StreamID, 128) || (request.ViewerID != "" && !safeTelemetryIdentifier(request.ViewerID, 128)) {
		return v2Error(c, fiber.StatusBadRequest, "invalid_telemetry", "Player telemetry identifiers were invalid.", false)
	}
	principal, _ := middleware.Principal(c)
	allowed, accessErr := database.Db.UserCanAccessChannel(c.UserContext(), principal.UserID, principal.Role, request.StreamID)
	if accessErr != nil || !allowed {
		return v2Error(c, fiber.StatusNotFound, "stream_not_found", "That stream session was not found.", false)
	}
	session, ok := streaming.DefaultManager.Get(request.StreamID)
	details, _ := json.Marshal(sanitizeTelemetryValue(request.Details, 0))
	if len(details) > 4096 {
		details = []byte(`{"truncated":true}`)
	}
	if request.Severity != "error" && request.Severity != "warning" {
		request.Severity = "info"
	}
	if request.Code == "" {
		request.Code = "player_event"
	}
	if !safeTelemetryIdentifier(request.Code, 64) {
		request.Code = "player_event"
	}
	if request.Message == "" {
		request.Message = "The player reported a stream event."
	}
	request.Message = truncateRunes(security.RedactSensitiveText(streaming.SanitizeDiagnostic(request.Message)), 500)
	if ok {
		session.RecordClientEvent(request.ViewerID, request.Severity, request.Code, request.Message, string(details))
		return c.JSON(fiber.Map{"incident_id": session.IncidentID(), "code": request.Code})
	}
	history, err := database.Db.GetLatestStreamSession(c.UserContext(), request.StreamID)
	if err != nil {
		return v2Error(c, fiber.StatusNotFound, "stream_not_found", "That stream session has ended.", false)
	}
	var connectionID *string
	if request.ViewerID != "" {
		connectionID = &request.ViewerID
	}
	if err := database.Db.InsertStreamEvent(c.UserContext(), models.StreamEvent{IncidentID: history.IncidentID,
		ConnectionID: connectionID, StreamID: request.StreamID, Severity: request.Severity,
		Code: request.Code, Message: request.Message, Details: string(details), CreatedAt: time.Now()}); err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "telemetry_failed", "Player telemetry could not be saved.", true)
	}
	return c.JSON(fiber.Map{"incident_id": history.IncidentID, "code": request.Code})
}

func safeTelemetryIdentifier(value string, maximum int) bool {
	if value == "" || len(value) > maximum {
		return false
	}
	for _, character := range value {
		if !(character == '-' || character == '_' || character >= '0' && character <= '9' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z') {
			return false
		}
	}
	return true
}

func truncateRunes(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) > maximum {
		return string(runes[:maximum])
	}
	return value
}

func sanitizeTelemetryValue(value any, depth int) any {
	if depth > 4 {
		return "[omitted]"
	}
	switch typed := value.(type) {
	case string:
		return truncateRunes(security.RedactSensitiveText(streaming.SanitizeDiagnostic(typed)), 500)
	case map[string]any:
		clean := make(map[string]any, min(len(typed), 64))
		count := 0
		for key, item := range typed {
			if count >= 64 {
				clean["truncated"] = true
				break
			}
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") || strings.Contains(lower, "csrf") {
				clean[key] = "[redacted]"
			} else {
				clean[key] = sanitizeTelemetryValue(item, depth+1)
			}
			count++
		}
		return clean
	case []any:
		maximum := min(len(typed), 64)
		clean := make([]any, 0, maximum)
		for _, item := range typed[:maximum] {
			clean = append(clean, sanitizeTelemetryValue(item, depth+1))
		}
		return clean
	default:
		return value
	}
}

// GetHlsChannels gets HLS channels by legacy template group.
// @Router /channels/hls/{group_id} [get]
func GetHlsChannels(c *fiber.Ctx) error {
	ctx := context.Background()
	groupID, err := strconv.ParseInt(c.Params("group_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	templateChannels, err := database.Db.GetTmplChannelsByGroup(groupID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "msg": "No channels found", "channels": nil})
	}

	channels := []models.LiveChannel{}
	for _, templateChannel := range templateChannels {
		items, itemErr := database.Db.GetTmplChannelItemsByCh(templateChannel.ID)
		if itemErr != nil || items == nil || len(*items) == 0 {
			continue
		}
		channel := models.LiveChannel{ID: templateChannel.ID, Name: templateChannel.Name}
		if logo, logoErr := database.Db.GetLogo(ctx, templateChannel.LogoId); logoErr == nil {
			channel.Logo = utils.GetLogoUrl(logo.Name)
		}
		channel.Stream = fmt.Sprintf("http://%s:%d/stream/hls/%s", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port, templateChannel.Uuid)
		if templateChannel.TvgID != nil {
			if programme, programmeErr := database.Db.GetCurrentProgramme(ctx, *templateChannel.TvgID, time.Now()); programmeErr == nil {
				channel.Programme = programme.Title.Value
				channel.Start = programme.Start.String()
				channel.End = programme.Stop.String()
				if next, nextErr := database.Db.GetCurrentProgramme(ctx, *templateChannel.TvgID, programme.Stop.Time); nextErr == nil {
					channel.Next = next.Title.Value
				} else {
					channel.Next = "No programme information"
				}
			} else {
				channel.Programme = "No programme information"
			}
		}
		channels = append(channels, channel)
	}

	return c.JSON(fiber.Map{"error": false, "msg": nil, "channels": channels})
}
