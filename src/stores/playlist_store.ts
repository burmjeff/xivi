import type { Playlist, PlaylistGroup, Match } from '@xivi/data/playlist_entities';
import { writable } from 'svelte/store';

export const playlists = writable<Playlist[]>([]);
export const playlistGroups = writable<PlaylistGroup[]>([]);
export const playlistMatches = writable<Match[]>([]);
