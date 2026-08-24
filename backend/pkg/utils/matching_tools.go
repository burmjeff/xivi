package utils

import (
	"database/sql"
	"strings"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/channelmatch"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

// Matching request queue with larger buffer
var (
	matchQueue     = make(chan matchRequest, 100)
	matchInit      sync.Once
	matchSemaphore = make(chan struct{}, 3)
)

type matchRequest struct {
	playlistCh *models.PlaylistChannel
	templateCh *models.TemplateChannel
	resultCh   chan<- error
}

const minimumAutoMatchMargin = 0.05

// Initialize the matching worker pool
func initMatchWorkers() {
	workerCount := 3

	for i := 0; i < workerCount; i++ {
		go matchWorker()
	}
}

// Worker that processes match requests
func matchWorker() {
	for req := range matchQueue {
		matchSemaphore <- struct{}{}

		var err error

		if req.playlistCh != nil {
			// Handle playlist channel matching
			if req.playlistCh.Enabled {
				if settings.APP_SETTINGS.Playlist.Tvgid_match {
					var matched bool
					matched, err = matchPlaylistTvgid(*req.playlistCh)
					if err == nil && matched {
						if req.resultCh != nil {
							req.resultCh <- nil
						}
						<-matchSemaphore
						continue
					}
				}

				if err == nil && settings.APP_SETTINGS.Playlist.Name_match {
					_, err = matchPlaylistChannelName(*req.playlistCh)
				}
			}
		} else if req.templateCh != nil {
			// Handle template channel matching
			if settings.APP_SETTINGS.Playlist.Tvgid_match {
				_, err = matchTemplateTvgid(req.templateCh)
			}

			// A template spans many playlists. Name matching still needs to run for
			// playlists where an exact ID was absent or ambiguous; it skips any
			// source already committed by the ID pass.
			if err == nil && settings.APP_SETTINGS.Playlist.Name_match {
				_, err = matchTemplateChannelName(req.templateCh)
			}
		}

		if req.resultCh != nil {
			req.resultCh <- err
		}

		<-matchSemaphore
	}
}

func MatchPlaylistChannel(playlistCh models.PlaylistChannel) {
	// Initialize workers if not already done
	matchInit.Do(initMatchWorkers)

	// Submit request to the matching queue
	matchQueue <- matchRequest{
		playlistCh: &playlistCh,
		templateCh: nil,
		resultCh:   nil, // No need for result in this case
	}
}

func MatchTemplateChannel(templateCh *models.TemplateChannel) {
	// Initialize workers if not already done
	matchInit.Do(initMatchWorkers)

	// Submit request to the matching queue
	matchQueue <- matchRequest{
		playlistCh: nil,
		templateCh: templateCh,
		resultCh:   nil, // No need for result in this case
	}
}

type sqlExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
}

const insertAutomaticMatchQuery = `INSERT INTO templatechannelitem (
	channel_id, playlist_channel_id, orderr, match_method, match_score,
	runner_up_score, matcher_version, manual_locked
) SELECT ?, ?, COALESCE(MAX(orderr), 0) + 1, ?, ?, ?, ?, false
	FROM templatechannelitem WHERE channel_id = ?
ON CONFLICT(channel_id, playlist_channel_id) DO NOTHING`

// Search if tvgid matches for playlist channel and add to template if match.
// The boolean distinguishes "matched" from "processed without a match" so name
// matching can run when a provider supplied a stale or unknown tvg-id.
func matchPlaylistTvgid(playlistCh models.PlaylistChannel) (bool, error) {
	if playlistCh.TvgID == nil || channelmatch.NormalizeTvgID(*playlistCh.TvgID) == "" {
		return false, nil
	}
	return retryPlaylistMatch("matchPlaylistTvgid", func() (bool, error) {
		return executeMatchPlaylistTvgid(playlistCh)
	})
}

func executeMatchPlaylistTvgid(playlistCh models.PlaylistChannel) (bool, error) {
	channels, err := database.Db.GetTmplChannelsBytvgid(playlistCh.TvgID)
	if err != nil {
		return false, err
	}
	if channels == nil || len(*channels) == 0 {
		return false, nil
	}

	selectedChannel := &(*channels)[0]
	matchMethod := channelmatch.MethodExactTvgID
	matchScore := 1.0
	runnerUpScore := 0.0
	if len(*channels) > 1 {
		results := channelmatch.Rank(playlistCh.Title, templateCandidates(*channels), len(*channels))
		selected := automaticResults(results)
		if len(selected) != 1 {
			return false, nil
		}
		selectedChannel = templateChannelByID(*channels, selected[0].Candidate.ID)
		if selectedChannel == nil {
			return false, nil
		}
		matchMethod = channelmatch.MethodTvgIDName
		matchScore = selected[0].Score
		runnerUpScore = selected[0].RunnerUpScore
	}

	group, err := database.Db.GetPlGroup(playlistCh.GroupId)
	if err != nil {
		return false, err
	}
	tx, err := database.Db.VectorQueries.Beginx()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	rejected, err := automaticMatchRejected(selectedChannel.ID, group.PlaylistId, playlistCh)
	if err != nil {
		return false, err
	}
	if !rejected {
		exists, existsErr := database.Db.TmplChannelExists(selectedChannel.ID, group.PlaylistId)
		if existsErr != nil {
			return false, existsErr
		}
		if !exists {
			if _, insertErr := insertAutomaticMatch(
				tx,
				selectedChannel.ID,
				playlistCh.ID,
				matchMethod,
				matchScore,
				runnerUpScore,
			); insertErr != nil {
				return false, insertErr
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func matchPlaylistChannelName(playlistCh models.PlaylistChannel) (bool, error) {
	if strings.TrimSpace(playlistCh.Title) == "" {
		return false, nil
	}
	return retryPlaylistMatch("matchPlaylistChannelName", func() (bool, error) {
		return executeMatchPlaylistChannelName(playlistCh)
	})
}

func executeMatchPlaylistChannelName(playlistCh models.PlaylistChannel) (bool, error) {
	templateChannels, err := database.Db.GetAllTmplChannels()
	if err != nil {
		return false, err
	}
	results := channelmatch.Rank(playlistCh.Title, templateCandidates(templateChannels), len(templateChannels))
	selected := automaticResults(results)
	if len(selected) == 0 {
		return false, nil
	}

	group, err := database.Db.GetPlGroup(playlistCh.GroupId)
	if err != nil {
		return false, err
	}
	tx, err := database.Db.VectorQueries.Beginx()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	matched := false
	for _, result := range selected {
		rejected, rejectionErr := automaticMatchRejected(result.Candidate.ID, group.PlaylistId, playlistCh)
		if rejectionErr != nil {
			return false, rejectionErr
		}
		if rejected {
			continue
		}
		exists, existsErr := database.Db.TmplChannelExists(result.Candidate.ID, group.PlaylistId)
		if existsErr != nil {
			return false, existsErr
		}
		if exists {
			continue
		}
		inserted, insertErr := insertAutomaticMatch(
			tx,
			result.Candidate.ID,
			playlistCh.ID,
			result.Method,
			result.Score,
			result.RunnerUpScore,
		)
		if insertErr != nil {
			return false, insertErr
		}
		matched = matched || inserted
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return matched, nil
}

// Search if tvgid matches for template channel and add one source per playlist.
func matchTemplateTvgid(templateCh *models.TemplateChannel) (bool, error) {
	if templateCh.TvgID == nil || channelmatch.NormalizeTvgID(*templateCh.TvgID) == "" {
		return false, nil
	}
	channels, err := database.Db.GetPlChannelsByTvgID(*templateCh.TvgID)
	if err != nil {
		return false, err
	}
	if len(channels) == 0 {
		return false, nil
	}

	channelsByPlaylist := make(map[int64][]models.PlaylistChannel)
	playlistIDs := make([]int64, 0)
	for _, channel := range channels {
		group, groupErr := database.Db.GetPlGroup(channel.GroupId)
		if groupErr != nil {
			return false, groupErr
		}
		if _, seen := channelsByPlaylist[group.PlaylistId]; !seen {
			playlistIDs = append(playlistIDs, group.PlaylistId)
		}
		channelsByPlaylist[group.PlaylistId] = append(channelsByPlaylist[group.PlaylistId], channel)
	}

	tx, err := database.Db.VectorQueries.Beginx()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	resolved := false
	for _, playlistID := range playlistIDs {
		playlistChannels := channelsByPlaylist[playlistID]
		selectedChannel := &playlistChannels[0]
		matchMethod := channelmatch.MethodExactTvgID
		matchScore := 1.0
		runnerUpScore := 0.0
		if len(playlistChannels) > 1 {
			results := channelmatch.Rank(templateCh.Name, playlistCandidates(playlistChannels), len(playlistChannels))
			selected := automaticResults(results)
			if len(selected) != 1 {
				continue
			}
			selectedChannel = playlistChannelByID(playlistChannels, selected[0].Candidate.ID)
			if selectedChannel == nil {
				continue
			}
			matchMethod = channelmatch.MethodTvgIDName
			matchScore = selected[0].Score
			runnerUpScore = selected[0].RunnerUpScore
		}

		rejected, rejectionErr := automaticMatchRejected(templateCh.ID, playlistID, *selectedChannel)
		if rejectionErr != nil {
			return false, rejectionErr
		}
		if rejected {
			resolved = true
			continue
		}
		exists, existsErr := database.Db.TmplChannelExists(templateCh.ID, playlistID)
		if existsErr != nil {
			return false, existsErr
		}
		if exists {
			resolved = true
			continue
		}
		if _, insertErr := insertAutomaticMatch(
			tx,
			templateCh.ID,
			selectedChannel.ID,
			matchMethod,
			matchScore,
			runnerUpScore,
		); insertErr != nil {
			return false, insertErr
		}
		resolved = true
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}
	return resolved, nil
}

func matchTemplateChannelName(templateCh *models.TemplateChannel) (bool, error) {
	if strings.TrimSpace(templateCh.Name) == "" {
		return false, nil
	}
	playlists, err := database.Db.GetPlaylists()
	if err != nil {
		return false, err
	}

	tx, err := database.Db.VectorQueries.Beginx()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	matched := false
	for _, playlist := range *playlists {
		exists, existsErr := database.Db.TmplChannelExists(templateCh.ID, playlist.ID)
		if existsErr != nil {
			return false, existsErr
		}
		if exists {
			continue
		}

		playlistChannels, channelsErr := database.Db.GetEnabledPlChannels(playlist.ID)
		if channelsErr != nil {
			return false, channelsErr
		}
		results := channelmatch.Rank(templateCh.Name, playlistCandidates(playlistChannels), len(playlistChannels))
		selected := automaticResults(results)
		if len(selected) == 0 {
			continue
		}

		// A template receives at most one source from each playlist.
		result := selected[0]
		playlistChannel := playlistChannelByID(playlistChannels, result.Candidate.ID)
		if playlistChannel == nil {
			continue
		}
		rejected, rejectionErr := automaticMatchRejected(templateCh.ID, playlist.ID, *playlistChannel)
		if rejectionErr != nil {
			return false, rejectionErr
		}
		if rejected {
			continue
		}
		inserted, insertErr := insertAutomaticMatch(
			tx,
			templateCh.ID,
			result.Candidate.ID,
			result.Method,
			result.Score,
			result.RunnerUpScore,
		)
		if insertErr != nil {
			return false, insertErr
		}
		matched = matched || inserted
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}
	return matched, nil
}

func retryPlaylistMatch(name string, operation func() (bool, error)) (bool, error) {
	var matched bool
	var err error
	retryDelay := 200 * time.Millisecond
	for attempt := 0; attempt < 3; attempt++ {
		matched, err = operation()
		if err == nil {
			return matched, nil
		}
		if attempt < 2 {
			log.Warn().Err(err).Int("attempt", attempt+1).Msg(name + " failed; retrying")
			time.Sleep(retryDelay)
			retryDelay *= 2
		}
	}
	return false, err
}

func automaticResults(results []channelmatch.Result) []channelmatch.Result {
	threshold := settings.APP_SETTINGS.Playlist.Name_score
	if threshold <= 0 || threshold > 1 {
		threshold = 0.96
	}
	return channelmatch.Automatic(results, threshold, minimumAutoMatchMargin)
}

func templateCandidates(channels []models.TemplateChannel) []channelmatch.Candidate {
	candidates := make([]channelmatch.Candidate, 0, len(channels))
	for _, channel := range channels {
		candidates = append(candidates, channelmatch.Candidate{ID: channel.ID, Name: channel.Name})
	}
	return candidates
}

func templateChannelByID(channels []models.TemplateChannel, id int64) *models.TemplateChannel {
	for index := range channels {
		if channels[index].ID == id {
			return &channels[index]
		}
	}
	return nil
}

func playlistCandidates(channels []models.PlaylistChannel) []channelmatch.Candidate {
	candidates := make([]channelmatch.Candidate, 0, len(channels))
	for _, channel := range channels {
		if channel.Enabled {
			candidates = append(candidates, channelmatch.Candidate{ID: channel.ID, Name: channel.Title})
		}
	}
	return candidates
}

func playlistChannelByID(channels []models.PlaylistChannel, id int64) *models.PlaylistChannel {
	for index := range channels {
		if channels[index].ID == id {
			return &channels[index]
		}
	}
	return nil
}

func automaticMatchRejected(
	templateChannelID int64,
	playlistID int64,
	playlistChannel models.PlaylistChannel,
) (bool, error) {
	return database.Db.ChannelMatchRejected(
		templateChannelID,
		playlistID,
		playlistChannel.TvgID,
		playlistChannel.Title,
	)
}

func insertAutomaticMatch(
	executor sqlExecutor,
	templateChannelID int64,
	playlistChannelID int64,
	method string,
	score float64,
	runnerUpScore float64,
) (bool, error) {
	result, err := executor.Exec(
		insertAutomaticMatchQuery,
		templateChannelID,
		playlistChannelID,
		method,
		score,
		runnerUpScore,
		channelmatch.CurrentVersion,
		templateChannelID,
	)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

func TopChannelMatches(templateCh *models.TemplateChannel) ([]models.VectorMatch, error) {
	playlistChannels, err := database.Db.GetAllEnabledPlChannels()
	if err != nil {
		return nil, err
	}

	ranked := channelmatch.Rank(templateCh.Name, playlistCandidates(playlistChannels), 5)
	matches := make([]models.VectorMatch, 0, len(ranked))
	for _, result := range ranked {
		matches = append(matches, models.VectorMatch{
			Id:    result.Candidate.ID,
			Score: result.Score,
		})
	}
	return matches, nil
}
