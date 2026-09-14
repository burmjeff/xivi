<script lang="ts">
	import legal from 'virtual:xivi-legal';
</script>

<svelte:head><title>About &amp; source · Xivi</title></svelte:head>

<section class="about">
	<a href="/">← Back to Xivi</a>
	<h1>About Xivi</h1>
	<p>Your channels. One guide.</p>
	<p>Copyright © 2023–2026 Xivi contributors. Licensed under GNU AGPL version 3 only.</p>
	<p>
		You may redistribute and modify Xivi under that license. Xivi comes without any warranty,
		including merchantability or fitness for a particular purpose.
	</p>
	<a class="source" href={legal.sourceUrl} target="_blank" rel="noopener noreferrer"
		>{legal.sourceLabel} ↗</a
	>
	<p class="hint">
		For modified deployments, the operator must offer the corresponding source for the version you
		use. The upstream project link alone does not include an operator's changes.
	</p>
	<details>
		<summary>GNU Affero General Public License v3.0</summary>
		<pre>{legal.license}</pre>
	</details>
	<details>
		<summary>Xivi copyright and notices</summary>
		<pre>{legal.notice}</pre>
	</details>
	<details>
		<summary>Third-party software</summary>
		<pre>{legal.thirdParty}</pre>
	</details>
	<details>
		<summary>Web dependency licenses and notices</summary>
		<pre>{legal.dependencyNotices}</pre>
	</details>
</section>

<style>
	.about {
		max-width: 56rem;
		margin: 0 auto;
		padding: 3rem 1.5rem 6rem;
		color: var(--text);
	}
	h1 {
		margin-top: 2rem;
		font-size: clamp(2rem, 6vw, 3.5rem);
	}
	p {
		margin-block: 1rem;
		line-height: 1.65;
	}
	a {
		color: var(--text);
		text-decoration: underline;
		text-underline-offset: 0.2em;
	}
	.source {
		display: inline-block;
		padding: 0.8rem 1rem;
		border: 1px solid var(--line);
		border-radius: 0.75rem;
	}
	.hint {
		color: var(--muted);
		font-size: 0.9rem;
	}
	details {
		border-top: 1px solid var(--line);
		padding-block: 1rem;
	}
	summary {
		cursor: pointer;
		font-weight: 700;
	}
	pre {
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		font-family: inherit;
		font-size: 0.9rem;
		line-height: 1.65;
		padding-block: 1rem;
	}
</style>
