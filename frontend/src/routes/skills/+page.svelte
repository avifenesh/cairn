<script lang="ts">
	import { onMount } from 'svelte';
	import { getSkills } from '$lib/api/client';
	import type { Skill } from '$lib/types';
	import { Sparkles, ToggleLeft, ToggleRight } from '@lucide/svelte';

	let skills = $state<Skill[]>([]);
	let activeSkills = $state<string[]>([]);
	let loading = $state(true);

	onMount(async () => {
		try {
			const res = await getSkills();
			skills = res.items;
			activeSkills = res.currentlyActive ?? [];
		} catch {
			// handled
		} finally {
			loading = false;
		}
	});
</script>

<div class="mx-auto max-w-4xl p-6">
	<h1 class="mb-6 text-2xl font-semibold text-[var(--text-primary)]">Skills</h1>

	{#if loading}
		<div class="flex flex-col gap-3">
			{#each Array(6) as _}
				<div class="h-16 animate-pulse rounded-lg bg-[var(--bg-2)]"></div>
			{/each}
		</div>
	{:else if skills.length === 0}
		<div class="flex flex-col items-center justify-center py-16 text-[var(--text-tertiary)]">
			<Sparkles class="mb-3 h-10 w-10 opacity-40" />
			<p class="text-sm">No skills loaded</p>
		</div>
	{:else}
		<div class="flex flex-col gap-2">
			{#each skills as skill (skill.name)}
				{@const isActive = activeSkills.includes(skill.name)}
				<div class="flex items-center gap-3 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3">
					{#if isActive}
						<ToggleRight class="h-5 w-5 flex-shrink-0 text-[var(--color-success)]" />
					{:else}
						<ToggleLeft class="h-5 w-5 flex-shrink-0 text-[var(--text-tertiary)]" />
					{/if}
					<div class="min-w-0 flex-1">
						<p class="text-sm font-medium text-[var(--text-primary)]">{skill.name}</p>
						<p class="truncate text-xs text-[var(--text-secondary)]">{skill.description}</p>
					</div>
					<span class="rounded-full bg-[var(--bg-3)] px-2 py-0.5 text-[10px] text-[var(--text-tertiary)]">
						{skill.scope}
					</span>
				</div>
			{/each}
		</div>
	{/if}
</div>
