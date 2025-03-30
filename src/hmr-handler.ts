// Custom HMR handler for Svelte components
if (import.meta.hot) {
  // Log when HMR is active
  console.log('HMR is active');

  // Listen for HMR events
  import.meta.hot.on('vite:beforeUpdate', (data) => {
    console.log('About to update with:', data);
  });

  import.meta.hot.on('vite:afterUpdate', (data) => {
    console.log('Updated with:', data);
  });

  import.meta.hot.on('vite:error', (data) => {
    console.error('HMR error:', data);
  });
}

export default {};
