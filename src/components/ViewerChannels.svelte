<!-- ViewerChannels.svelte -->

<script lang="ts">
	import { onMount } from 'svelte';
	import {
		Progress} from '@skeletonlabs/skeleton-svelte';
	import Player from './modals/Player.svelte';
	import { templates } from '@xivi/stores/template_store';
	import type { ViewerChannel } from '@xivi/data/viewer_entities';
	import Icon from '@iconify/svelte';

	interface Props {
		templateIdx: number;
		groupId: number;
		groupIdx: number;
	}

	let { templateIdx, groupId, groupIdx }: Props = $props();

	let playerModalOpen = $state(false);
	let currentPlayerName = $state('');
	let currentPlayerStream = $state('');

	const updateChannels = async () => {
		const response = await fetch(`/api/channels/hls/${groupId}`);
		const data = await response.json();
		return data.channels;
	};

	onMount(async () => {
		$templates[templateIdx].groups[groupIdx].viewerChannels = [];
		const fetchedData = await updateChannels();
		if (typeof fetchedData !== 'undefined') {
			fetchedData.forEach(function (channel: ViewerChannel) {
				$templates[templateIdx].groups[groupIdx].viewerChannels.push(channel);
				$templates[templateIdx].groups[groupIdx].viewerChannels =
					$templates[templateIdx].groups[groupIdx].viewerChannels;
			});
		}
	});

	function getProgress(start: string, end: string) {
		const now = Date.now();

		// Parse the custom date format: "2025-04-01 22:00:00 -0400 -0400"
		const parseCustomDate = (dateStr: string): number => {
			try {
				// Handle the specific format with duplicate timezone
				// Example: "2025-04-01 22:00:00 -0400 -0400"
				const regex = /(\d{4}-\d{2}-\d{2})\s(\d{2}:\d{2}:\d{2})\s(-\d{4})\s.*$/;
				const match = dateStr.match(regex);

				if (match) {
					const [_, datePart, timePart, timezone] = match;
					// Create a standard ISO format with the timezone
					const isoString = `${datePart}T${timePart}${timezone}`;
					console.log('Parsed date string:', isoString);
					return new Date(isoString).getTime();
				} else {
					// Fallback to simpler parsing if regex doesn't match
					const parts = dateStr.split(' ');
					if (parts.length >= 3) {
						// Just use the date and time parts
						const simpleFormat = `${parts[0]}T${parts[1]}`;
						console.log('Simple parsed date:', simpleFormat);
						return new Date(simpleFormat).getTime();
					}
				}
			} catch (error) {
				console.error('Error parsing date:', dateStr, error);
			}
			return NaN;
		};

		// Normal parsing
		const startTime = parseCustomDate(start);
		const endTime = parseCustomDate(end);

		// Check for invalid dates or equal start/end times
		if (isNaN(startTime) || isNaN(endTime) || startTime === endTime) {
			console.warn('Invalid date values or equal start/end times');
			return 0;
		}

		// If current time is before start time, return 0%
		if (now < startTime) {
			console.log('Program has not started yet');
			return 0;
		}

		// If current time is after end time, return 100%
		if (now > endTime) {
			console.log('Program has already ended');
			return 100;
		}

		// Calculate progress percentage
		let progress = Math.round(((now - startTime) / (endTime - startTime)) * 100);
		console.log('Progress calculation:', progress);

		// Ensure progress is between 0 and 100
		return Math.max(0, Math.min(100, progress));
	}

	// Simple function to open player modal with a stream
	function modalPlayer(name: string, stream: string) {
		// Set stream info
		currentPlayerName = name;
		currentPlayerStream = stream;

		// Close modal first if it's already open to ensure proper reset
		if (playerModalOpen) {
			playerModalOpen = false;

			// Use a delay to ensure modal is fully closed before reopening
			setTimeout(() => {
				playerModalOpen = true;
			}, 500);
		} else {
			// Just open the modal
			playerModalOpen = true;
		}
	}

	// Modal is closed via the onOpenChange event in the Player component
</script>

<Player
	modalOpen={playerModalOpen}
	name={currentPlayerName}
	stream={currentPlayerStream}
/>

{#if $templates[templateIdx].groups[groupIdx].viewerChannels != null}
	<section class="channels grid grid-cols-2 gap-2 p-1">
		{#each $templates[templateIdx].groups[groupIdx].viewerChannels as channel (channel.id)}
			<div
				class="channel w-content max-w-content card preset-filled-surface-100-900 preset-outlined-primary-500 border border-primary-900 card-hover grid h-32 grid-cols-5"
			>
				<img class="h-auto max-h-32 w-auto self-center p-4" src={channel.logo} alt="Logo" />
				<div class="col-span-3 mb-1 ml-4 mr-4 mt-1 self-center">
					<span class="h4 font-bold text-zinc-300 drop-shadow-md">
						{channel.name}
					</span>
					<div class="text-zinc-300">
						{channel.programme}
					</div>
					{#if channel.start != '' && channel.end != ''}
						<div class="flex w-full mt-2 drop-shadow-md">
							<Progress
								value={getProgress(channel.start, channel.end)}
								max={100}
								meterBg="preset-filled-primary-500"
								trackBg="preset-filled-surface-900-100"
								height="h-2"
							>{getProgress(channel.start, channel.end)}%</Progress>
						</div>
					{/if}
				</div>
				<button
					class="btn btn-md h-fit w-fit self-center"
					onclick={() => {
						modalPlayer(channel.name, channel.stream);
					}}
				>
					<Icon icon="icon-park-solid:play-two" width="50" height="50" />
				</button>
			</div>
		{/each}
	</section>
{:else}
	<p>No channels found</p>
{/if}
