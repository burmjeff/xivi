import type { Logo } from '@xivi/data/logo_entities';
import { writable } from 'svelte/store';

export const logos = writable<Logo[]>([]);
