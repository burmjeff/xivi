export interface Template {
	id: number;
	name: string;
	itemOpen: boolean;
}

export interface TemplateGroup {
	id: number;
	name: string;
	isDndShadowItem: boolean;
	itemOpen: boolean;
	channels: TemplateChannel[];
}	

export interface TemplateChannel {
	id: number;
	name: string;
    tvgid: string;
    logoid: number;
    uuid: string;
	logo: string;
}