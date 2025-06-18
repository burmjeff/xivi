// Custom HMR handler for Svelte components
if (import.meta.hot) {
	// Log when HMR is active
	console.log('HMR is active');

	// Listen for HMR events
	import.meta.hot.on('vite:beforeUpdate', (data: any) => {
		console.log('About to update with:', data);
	});

	import.meta.hot.on('vite:afterUpdate', (data: any) => {
		console.log('Updated with:', data);
	});

	import.meta.hot.on('vite:error', (data: any) => {
		console.error('HMR error:', data);
	});
}

export default {};
