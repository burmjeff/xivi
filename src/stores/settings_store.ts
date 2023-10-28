import type { AppSettings } from '@xivi/data/settings_entities';
import { writable } from 'svelte/store';

export const settings = writable<AppSettings>();