export interface Playlist {
	id: number;
	name: string;
}

export interface PlaylistGroup {
	id: number;
	name: string;
}

export interface PlaylistChannel {
	id: number;
	name: string;
    tvgid: string;
    tvg_logo: string;
    group_id: number;
    enabled: boolean;
}