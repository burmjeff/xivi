import type { ViewerChannel } from '@xivi/data/viewer_entities';
import { writable } from 'svelte/store';

export const channels = writable<ViewerChannel[]>([]);