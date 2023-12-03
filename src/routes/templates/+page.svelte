<script>
    import Template from '@xivi/components/Template.svelte';
    import TemplateGroupList from '@xivi/components/Template_Group_List.svelte';
    import Playlist from '@xivi/components/Playlist.svelte';
</script>

<div class="templates p-4 text-center justify-center items-center h-full w-full">
    <section class="grid grid-cols-2 gap-4 h-full w-full">
        <Template/>
        <TemplateGroupList/>
    </section>
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
