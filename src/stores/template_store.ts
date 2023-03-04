import type { Template } from '@xivi/data/template_entities';
import type { TemplateGroup } from '@xivi/data/template_entities';
import type { TemplateChannel } from '@xivi/data/template_entities';
import { writable } from 'svelte/store';

export const templates = writable<Template[]>([]);
export const templateGroups = writable<TemplateGroup[]>([]);
export const templateChannels = writable<TemplateChannel[]>([]);
