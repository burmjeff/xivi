export interface AppSettings {
	application: Application;
	server: Server;
	playlist: Playlist;
	streaming: Streaming;
}

export interface Application {
	appname: string;
	appversion: string;
	tz: string;
	servepath: string;
	loglevel: number;
	updatecron: string;
	ssdp: boolean;
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
