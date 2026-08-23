<script lang="ts">
	import { cva, type VariantProps } from 'class-variance-authority';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	const buttonVariants = cva(
		'inline-flex items-center justify-center rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none ring-offset-background',
		{
			variants: {
				variant: {
					default: 'bg-primary-500 text-primary-contrast-500 hover:bg-primary-600',
					destructive: 'bg-error-500 text-error-contrast-500 hover:bg-error-600',
					outline: 'border border-surface-500 hover:bg-surface-700 hover:text-surface-50',
					secondary: 'bg-secondary-500 text-secondary-contrast-500 hover:bg-secondary-600',
					ghost: 'hover:bg-surface-700 hover:text-surface-50',
					link: 'underline-offset-4 hover:underline text-primary-500'
				},
				size: {
					default: 'h-10 py-2 px-4',
					sm: 'h-9 px-3 rounded-md',
					lg: 'h-11 px-8 rounded-md',
					icon: 'h-10 w-10'
				}
			},
			defaultVariants: {
				variant: 'default',
				size: 'default'
			}
		}
	);

	type Props = {
		href?: string;
		variant?: VariantProps<typeof buttonVariants>['variant'];
		size?: VariantProps<typeof buttonVariants>['size'];
		class?: string;
		children?: import('svelte').Snippet;
		[key: string]: any;
	};

	let { class: className, variant, size, href, children, ...rest }: Props = $props();
</script>

{#if href}
	<a {href} class={buttonVariants({ variant, size, className })} {...rest}>
		{@render children?.()}
	</a>
{:else}
	<button class={buttonVariants({ variant, size, className })} {...rest}>
		{@render children?.()}
	</button>
{/if}
