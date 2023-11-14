export interface Playlist {
	id: number;
	name: string;
	url: string;
	itemOpen: boolean;
	playlistGroups: PlaylistGroup[];
}

export interface PlaylistGroup {
	id: string;
	name: string;
	isDndShadowItem: boolean;
	isDragged: boolean;
	playlistChannels: PlaylistChannel[];
}

export interface PlaylistChannel {
	id: number;
	title: string;
    tvg_id: string;
    tvg_logo: string;
    enabled: boolean;
}