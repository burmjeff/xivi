import { writable } from 'svelte/store';
import type { TemplateGroup } from '@xivi/data/template_entities';
import type { PlaylistGroup } from '@xivi/data/playlist_entities';

export const draggedItem = writable<PlaylistGroup | TemplateGroup | null>(null);