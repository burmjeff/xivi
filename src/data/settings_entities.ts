export interface AppSettings{
	application: Application
	server: Server
	streaming: Streaming
}

export interface Application {
	appname: string;
	appversion: string;
	tz: string;
	servepath: string;
	model: string;
	loglevel: number;
	updatecron: string;
}

export interface Server {
	host: string;
	port: number;
	readtimeout: number;
}

export interface Streaming {
	proxy: boolean;
	buffer: number;
	useragent: string;
}
