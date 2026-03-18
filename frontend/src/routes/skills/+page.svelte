<script lang="ts">
	import { onMount } from 'svelte';
	import { getSkills } from '$lib/api/client';
	import type { Skill } from '$lib/types';
	import { Badge } from '$lib/components/ui/badge';
	import { Skeleton } from '$lib/components/ui/skeleton';
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

<div class="mx-auto max-w-5xl p-6">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Skills</h1>
		{#if skills.length > 0}
			<span class="text-[11px] text-[var(--text-tertiary)] font-mono tabular-nums">
				{activeSkills.length} active / {skills.length} loaded
			</span>
		{/if}
	</div>

	{#if loading}
		<div class="flex flex-col gap-2">
			{#each Array(6) as _, i}
				<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3 animate-in" style="animation-delay: {i * 40}ms">
					<Skeleton class="h-4 w-32 mb-1" />
					<Skeleton class="h-3 w-48" />
				</div>
			{/each}
		</div>
	{:else if skills.length === 0}
		<div class="flex flex-col items-center justify-center py-20 text-[var(--text-tertiary)]">
			<Sparkles class="mb-3 h-10 w-10 opacity-30" />
			<p class="text-sm">No skills loaded</p>
			<p class="mt-1 text-xs opacity-60">Add SKILL.md files to your skill directories</p>
		</div>
	{:else}
		<div class="flex flex-col gap-2">
			{#each skills as skill, i (skill.name)}
				{@const isActive = activeSkills.includes(skill.name)}
				<div class="flex items-center gap-3 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3 card-hover animate-in" style="animation-delay: {i * 25}ms">
					{#if isActive}
						<ToggleRight class="h-5 w-5 flex-shrink-0 text-[var(--color-success)]" />
					{:else}
						<ToggleLeft class="h-5 w-5 flex-shrink-0 text-[var(--text-tertiary)]" />
					{/if}
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-2">
							<p class="text-sm font-medium text-[var(--text-primary)]">{skill.name}</p>
							{#if skill.userInvocable}
								<Badge variant="outline" class="h-4 px-1 text-[10px]">invocable</Badge>
							{/if}
						</div>
						<p class="truncate text-xs text-[var(--text-secondary)]">{skill.description}</p>
					</div>
					<Badge variant="secondary" class="h-5 text-[10px] flex-shrink-0">
						{skill.scope}
					</Badge>
				</div>
			{/each}
		</div>
	{/if}
</div>
