<!-- Settings.svelte -->

<script lang="ts">
	import { onMount } from 'svelte';
	import { settings } from '@xivi/stores/settings_store';
	import type { AppSettings } from '@xivi/data/settings_entities';
	import Icon from '@iconify/svelte';

	const getSettings = async () => {
		const response = await fetch('/api/settings');
		const data = await response.json();
		return data.settings;
	};

	onMount(async () => {
		const fetchedData = await getSettings();
		settings.set(fetchedData);
		(document.querySelector('.settings_appname input') as HTMLInputElement).value =
			$settings.application.appname;
		(document.querySelector('.settings_appversion input') as HTMLInputElement).value =
			$settings.application.appversion;
		(document.querySelector('.settings_tz input') as HTMLInputElement).value =
			$settings.application.tz;
		(document.querySelector('.settings_servepath input') as HTMLInputElement).value =
			$settings.application.servepath;
		(document.querySelector('.settings_loglevel input') as HTMLInputElement).value =
			$settings.application.loglevel.toString();
		(document.querySelector('.settings_updatecron input') as HTMLInputElement).value =
			$settings.application.updatecron;
		(document.querySelector('.settings_host input') as HTMLInputElement).value =
			$settings.server.host;
		(document.querySelector('.settings_port input') as HTMLInputElement).value =
			$settings.server.port.toString();
		(document.querySelector('.settings_readtimeout input') as HTMLInputElement).value =
			$settings.server.readtimeout.toString();
		(document.querySelector('.settings_type select') as HTMLInputElement).value =
			$settings.streaming.type.toString();
		(document.querySelector('.settings_proxy select') as HTMLInputElement).value =
			$settings.streaming.proxy.toString();
		(document.querySelector('.settings_buffer input') as HTMLInputElement).value =
			$settings.streaming.buffer.toString();
			(document.querySelector('.settings_retryeos input') as HTMLInputElement).value =
			$settings.streaming.retryeos.toString();
		(document.querySelector('.settings_useragent input') as HTMLInputElement).value =
			$settings.streaming.useragent;
		(document.querySelector('.settings_tvgmatch select') as HTMLInputElement).value =
			$settings.playlist.tvgid_match.toString();
		(document.querySelector('.settings_namematch select') as HTMLInputElement).value =
			$settings.playlist.name_match.toString();
		(document.querySelector('.settings_namescore input') as HTMLInputElement).value =
			$settings.playlist.name_score.toString();
	});

	async function updateSettings() {
		const inputAppName = (document.querySelector('.settings_appname input') as HTMLInputElement)
			.value;
		const inputAppVersion = (
			document.querySelector('.settings_appversion input') as HTMLInputElement
		).value;
		const inputTZ = (document.querySelector('.settings_tz input') as HTMLInputElement).value;
		const inputServePath = (document.querySelector('.settings_servepath input') as HTMLInputElement)
			.value;
		const inputLogLevel = (document.querySelector('.settings_loglevel input') as HTMLInputElement)
			.value;
		const inputUpdateCron = (
			document.querySelector('.settings_updatecron input') as HTMLInputElement
		).value;
		const inputHost = (document.querySelector('.settings_host input') as HTMLInputElement).value;
		const inputPort = (document.querySelector('.settings_port input') as HTMLInputElement).value;
		const inputReadTimeout = (
			document.querySelector('.settings_readtimeout input') as HTMLInputElement
		).value;
		const inputType = (document.querySelector('.settings_type select') as HTMLInputElement).value;
		const inputProxy = (document.querySelector('.settings_proxy select') as HTMLInputElement).value;
		const inputBuffer = (document.querySelector('.settings_buffer input') as HTMLInputElement).value;
		const inputRetryEOS = (document.querySelector('.settings_retryeos input') as HTMLInputElement).value;
		const inputUserAgent = (document.querySelector('.settings_useragent input') as HTMLInputElement).value;
		const inputTvgMatch = (document.querySelector('.settings_tvgmatch select') as HTMLInputElement).value;
		const inputNameMatch = (document.querySelector('.settings_namematch select') as HTMLInputElement).value;
		const inputNameScore = (document.querySelector('.settings_namescore input') as HTMLInputElement).value;
		if (
			inputAppName != '' &&
			inputAppVersion != '' &&
			inputTZ != '' &&
			inputServePath != '' &&
			inputLogLevel != '' &&
			inputUpdateCron != '' &&
			inputHost != '' &&
			inputPort != '' &&
			inputReadTimeout != '' &&
			inputType != '' &&
			inputProxy != '' &&
			inputBuffer != '' &&
			inputRetryEOS != '' &&
			inputUserAgent != ''
		) {
			let newSettings: AppSettings;
			newSettings = {
				application: {
					appname: inputAppName,
					appversion: inputAppVersion,
					tz: inputTZ,
					servepath: inputServePath,
					loglevel: Number(inputLogLevel),
					updatecron: inputUpdateCron
				},
				server: {
					host: inputHost,
					port: Number(inputPort),
					readtimeout: Number(inputReadTimeout)
				},
				playlist: {
					tvgid_match: inputTvgMatch === 'true',
					name_match: inputNameMatch === 'true',
					name_score: Number(inputNameScore)
				},
				streaming: {
					type: inputType,
					proxy: inputProxy === 'true',
					buffer: Number(inputBuffer),
					retryeos: Number(inputRetryEOS),
					useragent: inputUserAgent
				}
			};
			window.console.log('SETTINGS: ', newSettings);

			try {
				const response = await fetch('/api/settings', {
					method: 'PUT',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(newSettings)
				});
				const data = await response.status;
				console.log('Updated settings:', data);
			} catch (error) {
				console.log('Error updating settings:', error);
				return;
			}
		}
	}
</script>

<div class="card p-2">
	<h1 class="h1 justify-center text-center mb-4">Settings</h1>
	<div class="items-center space-y-4">
		<div class="application-viewport card drop-shadow-lg space-y-4 p-4">
			<h1 class="h3 justify-center text-start mb-2">Application</h1>
			<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
				<label class="settings_appname">
					<span>App Name</span>
					<input class="input variant-form-material" type="text" placeholder="App Name" />
				</label>
				<label class="settings_appversion">
					<span>App Version</span>
					<input class="input variant-form-material" type="text" placeholder="App Version" />
				</label>
				<label class="settings_tz">
					<span>Time Zone</span>
					<input class="input variant-form-material" type="text" placeholder="tz" />
				</label>
				<label class="settings_servepath">
					<span>File Serve Path (m3u/xml)</span>
					<input class="input variant-form-material" type="text" placeholder="./serve" />
				</label>
				<label class="settings_loglevel">
					<span>Log Level</span>
					<input class="input variant-form-material" type="number" placeholder="3" />
				</label>
				<label class="settings_updatecron">
					<span>Cron Update Schedule</span>
					<input class="input variant-form-material" type="text" placeholder="0 0 * * *" />
				</label>
			</div>
		</div>
		<div class="application-viewport card drop-shadow-lg space-y-4 p-4">
			<h1 class="h3 justify-center text-start mb-2">Server</h1>
			<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
				<label class="settings_host">
					<span>Server Host</span>
					<input class="input variant-form-material" type="text" placeholder="0.0.0.0" />
				</label>
				<label class="settings_port">
					<span>Server Port</span>
					<input class="input variant-form-material" type="number" placeholder="3000" />
				</label>
				<label class="settings_readtimeout">
					<span>Server Read Timeout</span>
					<input class="input variant-form-material" type="number" placeholder="60" />
				</label>
			</div>
		</div>
		<div class="application-viewport card drop-shadow-lg space-y-4 p-4">
			<h1 class="h3 justify-center text-start mb-2">Playlist</h1>
			<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
				<div class="w-full space-y-4">
					<label class="settings_tvgmatch">
						<span>Auto-Match by Tvg-Id</span>
						<select class="select">
							<option value="true">True</option>
							<option value="false">False</option>
						</select>
					</label>
				</div>
				<div class="w-full space-y-4">
					<label class="settings_namematch">
						<span>Auto-Match by Channel Name/Title</span>
						<select class="select">
							<option value="true">True</option>
							<option value="false">False</option>
						</select>
					</label>
				</div>
				<label class="settings_namescore">
					<span>Score to match Channel Name</span>
					<input class="input variant-form-material" type="number" step="0.01" placeholder="0" />
				</label>
			</div>
		</div>
		<div class="application-viewport card drop-shadow-lg space-y-4 p-4">
			<h1 class="h3 justify-center text-start mb-2">Streaming</h1>
			<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
				<div class="w-full space-y-4">
					<label class="settings_type">
						<span>Stream Type</span>
						<select class="select">
							<option value="hls">HLS</option>
							<option value="mp2t">MPEG-TS</option>
						</select>
					</label>
				</div>
				<div class="w-full space-y-4">
					<label class="settings_proxy">
						<span>Proxy Enabled</span>
						<select class="select">
							<option value="true">True</option>
							<option value="false">False</option>
						</select>
					</label>
				</div>
				<label class="settings_buffer">
					<span>Stream Buffer (Seconds)</span>
					<input class="input variant-form-material" type="number" placeholder="1" />
				</label>
				<label class="settings_retryeos">
					<span>Number of times to retry on end-of-stream received</span>
					<input class="input variant-form-material" type="number" placeholder="10" />
				</label>
				<label class="settings_useragent">
					<span>Proxy User-Agent</span>
					<input class="input variant-form-material" type="text" placeholder="Xivi 1.0" />
				</label>
			</div>
			<hr class="border-t-4!" />
			<button type="button" class="btn preset-filled-primary-500 items-center" onclick={updateSettings}>
				<Icon icon="icon-park-outline:save-one" width="18" height="18" />
				<span>Save Settings</span>
			</button>
		</div>
	</div>
</div>
