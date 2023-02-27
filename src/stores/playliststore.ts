import type { Playlist } from '../data/entities';
import type { PlaylistGroup } from '../data/entities';
import type { PlaylistChannel } from '../data/entities';
import { writable } from 'svelte/store';

export const playlists = writable<Playlist[]>([]);
export const playlistGroups = writable<PlaylistGroup[]>([]);
export const playlistChannels = writable<PlaylistChannel[]>([]);
