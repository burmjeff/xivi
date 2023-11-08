export interface Template {
	id: number;
	name: string;
}

export interface TemplateGroup {
	id: number;
	name: string;
	isDndShadowItem: boolean;
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