<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { getSkillDetail } from '$lib/api/client';
	import { renderMarkdown } from '$lib/utils/markdown';
	import { Badge } from '$lib/components/ui/badge';
	import { ArrowLeft, Loader2 } from '@lucide/svelte';

	const skillName = $page.params.name ?? '';

	let skill = $state<Record<string, unknown> | null>(null);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		try {
			skill = await getSkillDetail(skillName) as unknown as Record<string, unknown>;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load skill';
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>{skillName} - Skills - Cairn</title>
</svelte:head>

<div class="page-container">
	<header class="page-header">
		<a href="/skills" class="back-link">
			<ArrowLeft size={14} />
			<span>Skills</span>
		</a>
		<h1 class="skill-name">{skillName}</h1>
		{#if skill}
			<div class="skill-meta">
				<Badge variant="outline" class="text-xs">{skill.scope ?? 'global'}</Badge>
				<Badge variant={skill.inclusion === 'always' ? 'default' : 'secondary'} class="text-xs">{skill.inclusion ?? 'manual'}</Badge>
				{#if skill.disableModelInvocation}
					<Badge variant="destructive" class="text-xs">requires approval</Badge>
				{/if}
			</div>
		{/if}
	</header>

	{#if loading}
		<div class="center-state">
			<Loader2 size={24} class="animate-spin text-muted-foreground" />
		</div>
	{:else if error}
		<div class="center-state">
			<p class="text-sm text-destructive">{error}</p>
		</div>
	{:else if skill}
		{#if skill.description}
			<p class="skill-description">{skill.description}</p>
		{/if}
		{@const tools = Array.isArray(skill.allowedTools) ? skill.allowedTools as string[] : []}
		{#if tools.length > 0}
			<div class="allowed-tools">
				<span class="label">Allowed tools:</span>
				{#each tools as t}
					<Badge variant="outline" class="text-xs font-mono">{t}</Badge>
				{/each}
			</div>
		{/if}
		{#if skill.content}
			<div class="skill-content cairn-prose">
				{@html renderMarkdown(String(skill.content))}
			</div>
		{:else}
			<p class="text-sm text-muted-foreground">No content available</p>
		{/if}
	{/if}
</div>

<style>
	.page-container {
		padding: 1.5rem;
		max-width: 800px;
		margin: 0 auto;
	}
	.page-header {
		margin-bottom: 1rem;
	}
	.back-link {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.75rem;
		color: var(--text-tertiary);
		text-decoration: none;
		margin-bottom: 0.5rem;
	}
	.back-link:hover { color: var(--cairn-accent); }
	.skill-name {
		font-size: 1.125rem;
		font-weight: 600;
	}
	.skill-meta {
		display: flex;
		gap: 0.375rem;
		margin-top: 0.375rem;
	}
	.skill-description {
		font-size: 0.875rem;
		color: var(--text-secondary);
		margin-bottom: 1rem;
	}
	.allowed-tools {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.375rem;
		margin-bottom: 1rem;
	}
	.allowed-tools .label {
		font-size: 0.75rem;
		color: var(--text-tertiary);
	}
	.skill-content {
		font-size: 0.875rem;
		line-height: 1.6;
	}
	.center-state {
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 3rem;
	}

	/* Prose styles for skill markdown content */
	.cairn-prose :global(p) { margin: 0.5em 0; }
	.cairn-prose :global(code) {
		font-size: 0.8125rem; background: hsl(var(--muted) / 0.5);
		padding: 0.125rem 0.25rem; border-radius: 0.25rem;
	}
	.cairn-prose :global(pre) {
		background: hsl(var(--muted) / 0.5); border-radius: 0.375rem;
		padding: 0.75rem; margin: 0.5rem 0; overflow-x: auto;
		font-size: 0.8125rem; line-height: 1.4;
	}
	.cairn-prose :global(pre code) { background: none; padding: 0; }
	.cairn-prose :global(ul), .cairn-prose :global(ol) { padding-left: 1.25rem; margin: 0.5em 0; }
	.cairn-prose :global(li) { margin: 0.25em 0; }
	.cairn-prose :global(a) { color: var(--cairn-accent); text-decoration: underline; }
	.cairn-prose :global(h1), .cairn-prose :global(h2), .cairn-prose :global(h3) {
		font-weight: 600; margin: 1em 0 0.25em;
	}
	.cairn-prose :global(h1) { font-size: 1.125rem; }
	.cairn-prose :global(h2) { font-size: 1rem; }
	.cairn-prose :global(h3) { font-size: 0.9375rem; }
	.cairn-prose :global(blockquote) {
		border-left: 2px solid var(--cairn-accent);
		padding-left: 0.75rem; margin: 0.5em 0;
		color: var(--text-secondary);
	}
	.cairn-prose :global(table) { border-collapse: collapse; margin: 0.5rem 0; font-size: 0.8125rem; width: 100%; }
	.cairn-prose :global(th), .cairn-prose :global(td) {
		border: 1px solid hsl(var(--border)); padding: 0.375rem 0.5rem;
	}
	.cairn-prose :global(th) { background: hsl(var(--muted) / 0.3); font-weight: 600; }
</style>
