import type { Playlist, PlaylistGroup, Match } from '@xivi/data/playlist_entities';
import { writable } from 'svelte/store';

export const playlists = writable<Playlist[]>([]);
export const playlistgroups = writable<PlaylistGroup[]>([]);
export const playlistmatches = writable<Match[]>([]);