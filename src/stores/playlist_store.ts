import type { Playlist } from '../data/playlist_entities';
import type { PlaylistGroup } from '../data/playlist_entities';
import type { PlaylistChannel } from '../data/playlist_entities';
import { writable } from 'svelte/store';

export const playlists = writable<Playlist[]>([]);
export const playlistGroups = writable<PlaylistGroup[]>([]);
export const playlistChannels = writable<PlaylistChannel[]>([]);
