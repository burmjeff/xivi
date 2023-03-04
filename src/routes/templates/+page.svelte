<script>
    import Template from '../../components/Template.svelte';
    import TemplateGroupList from '../../components/Template_Group_List.svelte';
    import Playlist from '../../components/Playlist.svelte';
</script>

<div class="container h-full mx-auto flex justify-center items-center">
	<div class="space-y-10 text-center">
		<h2 class="font-bold">Welcome to Skeleton.</h2>
        <div style="display: flex;">
            <Template/>
            <TemplateGroupList/>
            <Playlist/>
        </div>
		<!-- / -->
		<div class="flex justify-center space-x-2">
			<a
				class="btn btn-filled"
				href="https://skeleton.dev/"
				target="_blank"
				rel="noreferrer"
			>
				Launch Documentation
			</a>
		</div>

		<div class="container mx-auto p-8 space-y-8">
			<h1>Hello Skeleton</h1>
			<section>
				<a class="btn variant-filled-primary" href="https://kit.svelte.dev/" target="_blank" rel="noreferrer">SvelteKit</a>
				<a class="btn variant-filled-secondary" href="https://tailwindcss.com/" target="_blank" rel="noreferrer">Tailwind</a>
				<a class="btn variant-filled-tertiary" href="https://github.com/" target="_blank" rel="noreferrer">GitHub</a>
			</section>
		</div>
		
		<div class="space-y-2">
			<p>Try editing the following:</p>
			<p><code>/src/routes/+layout.svelte</code></p>
			<p><code>/src/routes/+page.svelte</code></p>
		</div>
	</div>
</div>

<style lang="postcss">
	@keyframes glow {
		0% {
			@apply bg-primary-400/50;
		}
		33% {
			@apply bg-secondary-400/50;
		}
		66% {
			@apply bg-tertiary-400/50;
		}
		100% {
			@apply bg-primary-400/50;
		}
	}
	@keyframes pulse {
		50% {
			transform: scale(1.5);
		}
	}
</style>
