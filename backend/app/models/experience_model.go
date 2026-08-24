package models

import "time"

// LineupSummary is the compact representation used by Watch and Studio.
type LineupSummary struct {
	ID           int64   `db:"id" json:"id"`
	Name         string  `db:"name" json:"name"`
	GroupCount   int64   `db:"group_count" json:"group_count"`
	ChannelCount int64   `db:"channel_count" json:"channel_count"`
	EPGCoverage  float64 `db:"epg_coverage" json:"epg_coverage"`
}

type StudioGroupSummary struct {
	ID           int64            `db:"id" json:"id"`
	Name         string           `db:"name" json:"name"`
	Order        int64            `db:"orderr" json:"order"`
	ChannelCount int64            `db:"channel_count" json:"channel_count"`
	SourceLink   *SourceGroupLink `json:"source_link,omitempty"`
}

// SourceGroup is a selectable group from an ingested playlist source.
type SourceGroup struct {
	ID               int64  `db:"id" json:"id"`
	Name             string `db:"name" json:"name"`
	PlaylistID       int64  `db:"playlist_id" json:"playlist_id"`
	PlaylistName     string `db:"playlist_name" json:"playlist_name"`
	ChannelCount     int64  `db:"channel_count" json:"channel_count"`
	Enabled          bool   `db:"enabled" json:"enabled"`
	LinkedGroupCount int64  `db:"linked_group_count" json:"linked_group_count"`
}

// SourceGroupLink describes the one-way subscription owned by a lineup group.
type SourceGroupLink struct {
	GroupID            int64      `db:"group_id" json:"group_id"`
	PlaylistID         int64      `db:"playlist_id" json:"playlist_id"`
	PlaylistName       string     `db:"playlist_name" json:"playlist_name"`
	SourceGroupID      *int64     `db:"source_group_id" json:"source_group_id,omitempty"`
	SourceGroupName    string     `db:"source_group_name" json:"source_group_name"`
	FollowGroupName    bool       `db:"follow_group_name" json:"follow_group_name"`
	FollowChannelNames bool       `db:"follow_channel_names" json:"follow_channel_names"`
	Status             string     `db:"status" json:"status"`
	LastSyncedAt       *time.Time `db:"last_synced_at" json:"last_synced_at,omitempty"`
	LastError          string     `db:"last_error" json:"last_error,omitempty"`
	AddedCount         int64      `db:"added_count" json:"added_count"`
	UpdatedCount       int64      `db:"updated_count" json:"updated_count"`
	RemovedCount       int64      `db:"removed_count" json:"removed_count"`
}

type SourceGroupLinkRequest struct {
	SourceGroupID      int64 `json:"source_group_id"`
	FollowGroupName    bool  `json:"follow_group_name"`
	FollowChannelNames bool  `json:"follow_channel_names"`
}

type StudioGroupCreateRequest struct {
	Name       string                  `json:"name"`
	SourceLink *SourceGroupLinkRequest `json:"source_link,omitempty"`
}

type StudioGroupUpdateRequest struct {
	Name string `json:"name"`
}

type SourceGroupSyncResult struct {
	GroupID      int64 `json:"group_id"`
	AddedCount   int64 `json:"added_count"`
	UpdatedCount int64 `json:"updated_count"`
	RemovedCount int64 `json:"removed_count"`
}

// SourceGroupImportResult describes a one-time source-group copy. ChannelIDs
// stay internal so controllers can launch enrichment without returning a
// potentially large identifier array to the browser.
type SourceGroupImportResult struct {
	GroupID      int64   `json:"group_id"`
	GroupName    string  `json:"group_name"`
	AddedCount   int64   `json:"added_count"`
	SkippedCount int64   `json:"skipped_count"`
	ChannelIDs   []int64 `json:"-"`
}

// Programme uses correctly named start/end fields. The legacy EPG model has
// intentionally been left unchanged to keep old JSON output byte-compatible.
type Programme struct {
	ID          int64     `db:"id" json:"id"`
	ChannelID   string    `db:"channel" json:"channel_id"`
	Title       string    `db:"title" json:"title"`
	Subtitle    string    `db:"subtitle" json:"subtitle,omitempty"`
	Description string    `db:"description" json:"description,omitempty"`
	Categories  string    `db:"categories" json:"-"`
	Category    []string  `json:"categories"`
	Start       time.Time `db:"start" json:"start"`
	End         time.Time `db:"end" json:"end"`
}

type GuideChannel struct {
	ID         int64       `db:"id" json:"id"`
	Number     int64       `db:"number" json:"number"`
	Name       string      `db:"name" json:"name"`
	TVGID      *string     `db:"tvgid" json:"tvg_id,omitempty"`
	UUID       string      `db:"uuid" json:"-"`
	Logo       string      `db:"logo" json:"logo_url,omitempty"`
	GroupID    int64       `db:"group_id" json:"group_id"`
	GroupName  string      `db:"group_name" json:"group_name"`
	StreamURL  string      `json:"stream_url"`
	Programmes []Programme `json:"programmes"`
	Current    *Programme  `json:"current,omitempty"`
	Next       *Programme  `json:"next,omitempty"`
}

type WorkspaceChannel struct {
	ID            int64    `db:"id" json:"id"`
	Name          string   `db:"name" json:"name"`
	TVGID         *string  `db:"tvgid" json:"tvg_id,omitempty"`
	UUID          string   `db:"uuid" json:"uuid"`
	LogoID        int64    `db:"logo_id" json:"logo_id"`
	Logo          string   `db:"logo" json:"logo_url,omitempty"`
	Order         int64    `db:"orderr" json:"order"`
	SourceCount   int64    `db:"source_count" json:"source_count"`
	MatchMethod   string   `db:"match_method" json:"match_method"`
	MatchScore    *float64 `db:"match_score" json:"match_score,omitempty"`
	RunnerUpScore *float64 `db:"runner_up_score" json:"runner_up_score,omitempty"`
	ManualLocked  bool     `db:"manual_locked" json:"manual_locked"`
}

type SourceChannel struct {
	ID           int64   `db:"id" json:"id"`
	Name         string  `db:"name" json:"name"`
	TVGID        *string `db:"tvg_id" json:"tvg_id,omitempty"`
	LogoURL      *string `db:"logo_url" json:"logo_url,omitempty"`
	Enabled      bool    `db:"enabled" json:"enabled"`
	GroupID      int64   `db:"group_id" json:"group_id"`
	GroupName    string  `db:"group_name" json:"group_name"`
	PlaylistID   int64   `db:"playlist_id" json:"playlist_id"`
	PlaylistName string  `db:"playlist_name" json:"playlist_name"`
}

type MatchReview struct {
	ChannelID       int64    `db:"channel_id" json:"channel_id"`
	ChannelName     string   `db:"channel_name" json:"channel_name"`
	SourceChannelID int64    `db:"source_channel_id" json:"source_channel_id"`
	SourceName      string   `db:"source_name" json:"source_name"`
	PlaylistName    string   `db:"playlist_name" json:"playlist_name"`
	Method          string   `db:"method" json:"method"`
	Score           *float64 `db:"score" json:"score,omitempty"`
	RunnerUpScore   *float64 `db:"runner_up_score" json:"runner_up_score,omitempty"`
	ManualLocked    bool     `db:"manual_locked" json:"manual_locked"`
}

type MatchRejection struct {
	ID           int64     `db:"id" json:"id"`
	ChannelID    int64     `db:"channel_id" json:"channel_id"`
	PlaylistID   int64     `db:"playlist_id" json:"playlist_id"`
	PlaylistName string    `db:"playlist_name" json:"playlist_name"`
	TVGID        string    `db:"tvg_id_norm" json:"tvg_id"`
	Name         string    `db:"name_norm" json:"name"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

type StudioOverview struct {
	LineupCount        int64   `db:"lineup_count" json:"lineup_count"`
	SourceCount        int64   `db:"source_count" json:"source_count"`
	SourceChannelCount int64   `db:"source_channel_count" json:"source_channel_count"`
	LineupChannelCount int64   `db:"lineup_channel_count" json:"lineup_channel_count"`
	MappedChannelCount int64   `db:"mapped_channel_count" json:"mapped_channel_count"`
	ReviewCount        int64   `db:"review_count" json:"review_count"`
	LowConfidenceCount int64   `db:"low_confidence_count" json:"low_confidence_count"`
	UnmatchedCount     int64   `db:"unmatched_count" json:"unmatched_count"`
	EPGCoverage        float64 `db:"epg_coverage" json:"epg_coverage"`
	LogoCount          int64   `db:"logo_count" json:"logo_count"`
}

type CoverageSummary struct {
	EPGChannelCount      int64    `db:"epg_channel_count" json:"epg_channel_count"`
	ProgrammeCount       int64    `db:"programme_count" json:"programme_count"`
	MappedChannelCount   int64    `db:"mapped_channel_count" json:"mapped_channel_count"`
	LineupChannelCount   int64    `db:"lineup_channel_count" json:"lineup_channel_count"`
	Coverage             float64  `db:"coverage" json:"coverage"`
	UnmappedEPGIDs       []string `json:"unmapped_epg_ids"`
	ChannelsWithoutTVGID []string `json:"channels_without_tvg_id"`
}

type OperationJob struct {
	ID         int64      `db:"id" json:"id"`
	Kind       string     `db:"kind" json:"kind"`
	Resource   string     `db:"resource" json:"resource"`
	ResourceID *int64     `db:"resource_id" json:"resource_id,omitempty"`
	Status     string     `db:"status" json:"status"`
	Progress   int        `db:"progress" json:"progress"`
	Message    string     `db:"message" json:"message,omitempty"`
	ErrorCode  string     `db:"error_code" json:"error_code,omitempty"`
	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at" json:"updated_at"`
	FinishedAt *time.Time `db:"finished_at" json:"finished_at,omitempty"`
}

type Paginated[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
	Total      int64   `json:"total"`
}

type APIError struct {
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	FieldErrors map[string]string `json:"field_errors,omitempty"`
	Retryable   bool              `json:"retryable"`
}

type WorkspaceChannelUpdate struct {
	Name   string  `json:"name" validate:"required,lte=255"`
	TVGID  *string `json:"tvg_id"`
	LogoID *int64  `json:"logo_id"`
}

type SourceChannelEnableRequest struct {
	ChannelIDs []int64 `json:"channel_ids"`
	Enabled    bool    `json:"enabled"`
}

type WorkspaceBatchAddRequest struct {
	SourceChannelIDs []int64 `json:"source_channel_ids"`
}

type WorkspaceBatchRemoveRequest struct {
	ChannelIDs []int64 `json:"channel_ids"`
}

type WorkspaceBatchMoveRequest struct {
	ChannelIDs    []int64 `json:"channel_ids"`
	TargetGroupID int64   `json:"target_group_id"`
	BeforeID      *int64  `json:"before_id"`
	AfterID       *int64  `json:"after_id"`
}
