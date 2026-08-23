<script lang="ts">
	import '@xivi/app.css';
	import Header from '$lib/components/layout/Header.svelte';
	import Sidebar from '$lib/components/layout/Sidebar.svelte';

	let { children } = $props();
</script>

<!-- Modern Layout Structure -->
<div class="bg-surface-950 bg-grid-pattern text-surface-50 flex h-full flex-col overflow-hidden">
	<!-- Header -->
	<header class="z-20 shrink-0">
		<Header />
	</header>

	<div class="flex flex-1 overflow-hidden">
		<!-- Sidebar -->
		<aside
			class="border-surface-800/50 bg-surface-900/30 hidden w-64 shrink-0 border-r backdrop-blur-sm md:block"
		>
			<Sidebar />
		</aside>

		<!-- Main Content -->
		<main class="relative flex-1 overflow-x-hidden overflow-y-auto scroll-smooth">
			<div class="container mx-auto max-w-7xl p-6 lg:p-8">
				{@render children?.()}
			</div>

			<!-- Footer -->
			<footer
				class="border-surface-800/50 text-surface-400 mt-12 border-t py-6 text-center text-sm"
			>
				<div class="flex items-center justify-center gap-3">
					<span class="text-surface-300 font-medium">Xivi</span>
					<span class="text-surface-700">•</span>
					<span>v0.1.0</span>
					<span class="text-surface-700">•</span>
					<a
						href="https://github.com/burmjeff/xivi/issues"
						target="_blank"
						rel="noreferrer"
						class="hover:text-primary-400 transition-colors"
					>
						Report Issue
					</a>
				</div>
			</footer>
		</main>
	</div>
</div>
