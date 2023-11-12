export interface Playlist {
	id: number;
	name: string;
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
	name: string;
    tvgid: string;
    tvg_logo: string;
    enabled: boolean;
}