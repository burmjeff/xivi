export interface Template {
	id: number;
	name: string;
}

export interface TemplateGroup {
	id: number;
	name: string;
}

export interface TemplateChannel {
	id: number;
	name: string;
    tvgid: string;
    logo: string;
    uuid: string;
}