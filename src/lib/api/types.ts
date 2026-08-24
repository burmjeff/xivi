import type { components } from './generated';

// API v2 DTOs are generated from docs/openapi-v2.yaml. Components consume
// these named aliases and compose local view models without mirroring the wire contract.
export type LineupSummary = components['schemas']['LineupSummary'];
export type Programme = components['schemas']['Programme'];
export type GuideChannel = components['schemas']['GuideChannel'];
export type StudioGroup = components['schemas']['StudioGroup'];
export type WorkspaceChannel = components['schemas']['WorkspaceChannel'];
export type SourceChannel = components['schemas']['SourceChannel'];
export type MatchReview = components['schemas']['MatchReview'];
export type MatchRejection = components['schemas']['MatchRejection'];
export type OperationJob = components['schemas']['OperationJob'];
export type StudioOverview = components['schemas']['StudioOverview'];
export type CoverageSummary = components['schemas']['CoverageSummary'];
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
