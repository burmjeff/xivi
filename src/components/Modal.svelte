<script lang="ts">
	import { Dialog } from '@skeletonlabs/skeleton-svelte';
	import type { Snippet } from 'svelte';

	let {
		open = $bindable(),
		onOpenChange,
		contentBase = 'card bg-surface-800 p-0 shadow-xl max-w-screen-sm border border-surface-700/50 rounded-lg overflow-hidden w-full',
		backdropClasses = 'fixed inset-0 z-50 bg-black/50 backdrop-blur-sm',
		content
	}: {
		open: boolean;
		onOpenChange?: (e: { open: boolean }) => void;
		contentBase?: string;
		backdropClasses?: string;
		content: Snippet;
	} = $props();
</script>

<Dialog
	{open}
	onOpenChange={(e) => {
		open = e.open;
		if (onOpenChange) onOpenChange(e);
	}}
>
	<Dialog.Backdrop class={backdropClasses} />
	<Dialog.Positioner class="fixed inset-0 z-50 flex items-center justify-center p-4">
		<Dialog.Content class={contentBase}>
			{@render content()}
		</Dialog.Content>
	</Dialog.Positioner>
</Dialog>
