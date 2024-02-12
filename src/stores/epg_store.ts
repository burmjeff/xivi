import type { Epg } from '@xivi/data/epg_entities';
import { writable } from 'svelte/store';

export const epgs = writable<Epg[]>([]);
