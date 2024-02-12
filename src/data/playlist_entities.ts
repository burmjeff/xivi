export interface Playlist {
	id: number;
	name: string;
	url: string;
	itemOpen: boolean;
	groups: PlaylistGroup[];
}

export interface PlaylistGroup {
	id: string;
	name: string;
	isDndShadowItem: boolean;
	isDragged: boolean;
	playlistId: number;
	channels: PlaylistChannel[];
}

export interface PlaylistChannel {
	id: string;
	title: string;
	tvg_id: string;
	tvg_logo: string;
	enabled: boolean;
	isDndShadowItem: boolean;
	isDragged: boolean;
}

export interface Match {
	id: number;
	name: string;
	tvgid: string;
	score: number;
	isDndShadowItem: boolean;
	isDragged: boolean;
}
