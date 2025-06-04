/** @type {import('tailwindcss').Config} */
export default {
  content: [
    './src/**/*.{html,js,svelte,ts}',
    // Include Skeleton UI components
    './node_modules/@skeletonlabs/skeleton-svelte/**/*.{html,js,svelte,ts}',
    './node_modules/@skeletonlabs/skeleton/**/*.{html,js,svelte,ts}'
  ],
  theme: {
    extend: {},
  },
  plugins: [
    require('@tailwindcss/forms'),
    // Add Skeleton UI theme
    ...require('@skeletonlabs/skeleton/tailwind/skeleton.cjs')()
  ],
  darkMode: 'class'
}
