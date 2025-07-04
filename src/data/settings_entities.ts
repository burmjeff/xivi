export interface AppSettings {
	application: Application;
	server: Server;
	playlist: Playlist;
	streaming: Streaming;
	upnp: UPnP;
}

export interface Application {
	appname: string;
	appversion: string;
	tz: string;
	servepath: string;
	loglevel: number;
	updatecron: string;
}

export interface Server {
	host: string;
	port: number;
	readtimeout: number;
}

export interface Playlist {
	tvgid_match: boolean;
	name_match: boolean;
	name_score: number;
}

export interface Streaming {
	type: string;
	proxy: boolean;
	buffer: number;
	retryeos: number;
	useragent: string;
}

export interface UPnP {
	enabled: boolean;
	manufacturer: string;
	model_name: string;
	model_number: string;
	firmware_name: string;
	firmware_version: string;
	device_auth: string;
	tuner_count: number;
}
