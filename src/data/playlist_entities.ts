export interface Playlist {
	id: number;
	name: string;
	url: string;
	updated_at: string;
	itemOpen: boolean;
	groups: PlaylistGroup[];
}

export interface PlaylistGroup {
	id: string;
	name: string;
	playlist_id: number;
	enabled: boolean;
	isDndShadowItem: boolean;
	isDragged: boolean;
	channels: PlaylistChannel[];
}

export interface PlaylistChannel {
	id: string;
	title: string;
	tvg_id: string;
	tvg_logo: string;
	group_id: number;
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
