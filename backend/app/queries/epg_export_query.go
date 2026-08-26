package queries

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/settings"

	"github.com/jmoiron/sqlx"
)

const epgExportChannelBatchSize = 400

type epgExportProgrammeRow struct {
	ID                  int64          `db:"id"`
	Start               time.Time      `db:"start"`
	Stop                time.Time      `db:"stop"`
	Channel             string         `db:"channel"`
	TitleValue          sql.NullString `db:"title_value"`
	TitleLang           sql.NullString `db:"title_lang"`
	Subtitle            sql.NullString `db:"subtitle"`
	Description         sql.NullString `db:"description"`
	Categories          sql.NullString `db:"categories"`
	Icon                sql.NullString `db:"icon"`
	Directors           sql.NullString `db:"directors"`
	Presenters          sql.NullString `db:"presenters"`
	Producers           sql.NullString `db:"producers"`
	Actors              sql.NullString `db:"actors"`
	EpisodeNumberSystem sql.NullString `db:"episode_number_system"`
	EpisodeNumberValue  sql.NullString `db:"episode_number_value"`
	RatingSystem        sql.NullString `db:"rating_system"`
	RatingValue         sql.NullString `db:"rating_value"`
	VideoQuality        sql.NullString `db:"video_quality"`
	Date                sql.NullString `db:"date"`
}

// GetProgrammesByTVGIDsWindow loads an export window in bounded, set-based
// batches. This replaces the XMLTV exporter's previous query-per-channel path
// and prevents stale rows outside the window from suppressing placeholders.
func (q *EpgQueries) GetProgrammesByTVGIDsWindow(ctx context.Context, tvgIDs []string, from, to time.Time) ([]models.EpgProgramme, error) {
	uniqueIDs := make([]string, 0, len(tvgIDs))
	seen := make(map[string]struct{}, len(tvgIDs))
	for _, value := range tvgIDs {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		uniqueIDs = append(uniqueIDs, value)
	}
	if len(uniqueIDs) == 0 {
		return []models.EpgProgramme{}, nil
	}

	location, err := time.LoadLocation(settings.APP_SETTINGS.Application.TZ)
	if err != nil {
		location = time.UTC
	}
	from = from.In(location)
	to = to.In(location)
	programmes := make([]models.EpgProgramme, 0)
	for start := 0; start < len(uniqueIDs); start += epgExportChannelBatchSize {
		end := start + epgExportChannelBatchSize
		if end > len(uniqueIDs) {
			end = len(uniqueIDs)
		}
		query, args, err := sqlx.In(`
			SELECT id, start, stop, channel,
			       "title.value" AS title_value, "title.lang" AS title_lang,
			       subtitle, desc AS description, categories, "icon.src" AS icon,
			       directors, presenters, producers, actors,
			       "episodenumber.system" AS episode_number_system,
			       "episodenumber.value" AS episode_number_value,
			       "rating.system" AS rating_system, "rating.value" AS rating_value,
			       "video.quality" AS video_quality, date
			FROM epgprogramme
			WHERE channel IN (?) AND start < ? AND stop > ?
			ORDER BY channel, start`, uniqueIDs[start:end], to, from)
		if err != nil {
			return nil, err
		}
		rows := []epgExportProgrammeRow{}
		if err := q.SelectContext(ctx, &rows, q.Rebind(query), args...); err != nil {
			return nil, err
		}
		for _, row := range rows {
			programmes = append(programmes, exportProgramme(row, location))
		}
	}

	sort.SliceStable(programmes, func(i, j int) bool {
		if programmes[i].Channel != programmes[j].Channel {
			return programmes[i].Channel < programmes[j].Channel
		}
		return programmes[i].Start.Before(programmes[j].Start.Time)
	})
	return programmes, nil
}

func exportProgramme(row epgExportProgrammeRow, location *time.Location) models.EpgProgramme {
	title := "No Title"
	if row.TitleValue.Valid && strings.TrimSpace(row.TitleValue.String) != "" {
		title = row.TitleValue.String
	}
	return models.EpgProgramme{
		ID:      row.ID,
		Start:   &models.Time{Time: row.Start.In(location)},
		Stop:    &models.Time{Time: row.Stop.In(location)},
		Channel: row.Channel,
		Title: models.Title{
			Value: title,
			Lang:  nullableString(row.TitleLang),
		},
		Subtitle:   nullableString(row.Subtitle),
		Desc:       nullableString(row.Description),
		Categories: splitExportValues(row.Categories),
		Icon:       models.Icon{Src: nullableString(row.Icon)},
		Directors:  splitExportValues(row.Directors),
		Presenters: splitExportValues(row.Presenters),
		Producers:  splitExportValues(row.Producers),
		Actors:     splitExportValues(row.Actors),
		EpisodeNumber: models.EpisodeNumber{
			System: nullableString(row.EpisodeNumberSystem),
			Value:  nullableString(row.EpisodeNumberValue),
		},
		Rating: models.Rating{
			System: nullableString(row.RatingSystem),
			Value:  nullableString(row.RatingValue),
		},
		Video: models.Video{Quality: nullableString(row.VideoQuality)},
		Date:  nullableString(row.Date),
	}
}

func nullableString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func splitExportValues(value sql.NullString) models.StringArray {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return models.StringArray{}
	}
	parts := strings.Split(value.String, ",")
	values := make(models.StringArray, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			values = append(values, part)
		}
	}
	return values
}
