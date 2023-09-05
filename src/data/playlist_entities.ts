export interface Playlist {
	id: number;
	name: string;
	isDndShadowItem: boolean;
}

export interface PlaylistGroup {
	id: number;
	name: string;
	isDndShadowItem: boolean;
}

export interface PlaylistChannel {
	id: number;
	name: string;
    tvgid: string;
    tvg_logo: string;
    enabled: boolean;
	isDndShadowItem: boolean;
}