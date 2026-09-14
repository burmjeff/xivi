import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
	appId: 'com.xivi.app',
	appName: 'Xivi',
	webDir: 'build',
	server: {
		androidScheme: 'https',
		hostname: 'app.xivi.local'
	},
	android: {
		allowMixedContent: false,
		webContentsDebuggingEnabled: false
	},
	plugins: {
		SystemBars: {
			style: 'DARK'
		}
	}
};

export default config;
