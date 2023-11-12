export interface Template {
	id: string;
	name: string;
	itemOpen: boolean;
}

export interface TemplateGroup {
	id: string;
	name: string;
	isDndShadowItem: boolean;
	itemOpen: boolean;
	isDragged: boolean;
	channels: TemplateChannel[];
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