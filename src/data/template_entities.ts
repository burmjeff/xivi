import type { ViewerChannel } from './viewer_entities';

export interface Template {
	id: number;
	name: string;
	itemOpen: boolean;
	groups: TemplateGroup[];
}

export interface TemplateGroup {
	id: string;
	name: string;
	dynamic: boolean;
	dynamicgroup: number;
	isDndShadowItem: boolean;
	itemOpen: boolean;
	isDragged: boolean;
	channels: TemplateChannel[];
	viewerChannels: ViewerChannel[];
}

export interface TemplateChannel {
	id: string;
	name: string;
	tvgid: string;
	logoid: number;
	uuid: string;
	logo: string;
	isDndShadowItem: boolean;
	isDragged: boolean;
}
