import type { Playlist } from '@xivi/data/playlist_entities';
import type { PlaylistGroup } from '@xivi/data/playlist_entities';
import type { PlaylistChannel } from '@xivi/data/playlist_entities';
import { writable } from 'svelte/store';

export const playlists = writable<Playlist[]>([]);
export const playlistGroups = writable<PlaylistGroup[]>([]);
export const playlistChannels = writable<PlaylistChannel[]>([]);
