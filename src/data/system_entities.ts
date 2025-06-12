// System status interfaces
export interface SystemStatus {
	uptime: string;
	cpu: string;
	memory: string;
	connections: number;
	timestamp: number;
}

export interface SystemStatusResponse {
	error: boolean;
	msg: null | string;
	status: SystemStatus;
}

// Local system status state interface
export interface SystemStatusState {
	uptime: string;
	cpu: string;
	memory: string;
	connections: number;
	isLoading: boolean;
	error: string | null;
	lastUpdated: number | null;
}
