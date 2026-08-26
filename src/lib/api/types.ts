import type { components } from './generated';

// API v2 DTOs are generated from docs/openapi-v2.yaml. Components consume
// these named aliases and compose local view models without mirroring the wire contract.
export type LineupSummary = components['schemas']['LineupSummary'];
export type WatchGroupSummary = components['schemas']['WatchGroupSummary'];
export type Programme = components['schemas']['Programme'];
export type GuideChannel = components['schemas']['GuideChannel'];
export type WatchChannelNeighbors = components['schemas']['WatchChannelNeighbors'];
export type StudioGroup = components['schemas']['StudioGroup'];
export type SourceGroup = components['schemas']['SourceGroup'];
export type SourceGroupLink = components['schemas']['SourceGroupLink'];
export type SourceGroupImportResult = components['schemas']['SourceGroupImportResult'];
export type WorkspaceChannel = components['schemas']['WorkspaceChannel'];
export type SourceChannel = components['schemas']['SourceChannel'];
export type MatchReview = components['schemas']['MatchReview'];
export type MatchSuggestion = components['schemas']['MatchSuggestion'];
export type MatchRejection = components['schemas']['MatchRejection'];
export type OperationJob = components['schemas']['OperationJob'];
export type StudioOverview = components['schemas']['StudioOverview'];
export type CoverageSummary = components['schemas']['CoverageSummary'];
export type StreamingStatus = components['schemas']['StreamingStatus'];
export type StreamingSummary = components['schemas']['StreamingSummary'];
export type StreamingSession = components['schemas']['StreamingSession'];
export type StudioStream = components['schemas']['StudioStream'];
export type StudioStreamsResponse = components['schemas']['StudioStreamsResponse'];
export type StreamConnection = components['schemas']['StreamConnection'];
export type StreamMetricSample = components['schemas']['StreamMetricSample'];
export type StreamEvent = components['schemas']['StreamEvent'];
export type StreamSessionHistory = components['schemas']['StreamSessionHistory'];
export type APIError = components['schemas']['APIError'];

// Shared page view model, also used for legacy adapters outside API v2.
export interface Paginated<T> {
	items: T[];
	next_cursor: string | null;
	total: number;
}

export interface LegacyPlaylist {
	id: number;
	name: string;
	url: string;
	connection_limit: number;
	active_connections: number;
	created_at: string;
	updated_at: string;
}

export interface LegacyEpg {
	id: number;
	name: string;
	url: string;
	orderr: number;
	created_at: string;
	updated_at: string;
}

export interface LegacyLogo {
	id: number;
	name: string;
}
