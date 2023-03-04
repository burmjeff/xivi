import type { Template } from '../data/template_entities';
import type { TemplateGroup } from '../data/template_entities';
import type { TemplateChannel } from '../data/template_entities';
import { writable } from 'svelte/store';

export const templates = writable<Template[]>([]);
export const templateGroups = writable<TemplateGroup[]>([]);
export const templateChannels = writable<TemplateChannel[]>([]);
