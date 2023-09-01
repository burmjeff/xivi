<script>
    import Template from '@xivi/components/Template.svelte';
    import TemplateGroupList from '@xivi/components/Template_Group_List.svelte';
    import Playlist from '@xivi/components/Playlist.svelte';
</script>

<div class="container h-full mx-auto flex justify-center items-start">
	<div class="space-y-10 text-center">
		<h2 class="font-bold">Welcome to Skeleton.</h2>
        <div style="display: flex; max-w-fit">
            <Template/>
            <TemplateGroupList/>
            <Playlist/>
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

	.hovering {
        border-color: orange;
    }
    .item {
        display: inline; /* required for flip to work */
    }
    li {
        background-color: lightgray;
        cursor: pointer;
        display: inline-block;
        margin-right: 10px;
        padding: 10px;
    }
    li:hover {
        background: orange;
        color: white;
    }
  p {
        border: solid lightgray 1px;
        display: flex; /* required for drag & drop to work when .item display is inline */
        height: 40px; /* needed when empty */
        padding: 10px;
    }
</style>
