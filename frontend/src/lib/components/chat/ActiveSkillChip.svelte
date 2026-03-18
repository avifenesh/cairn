<script lang="ts">
	import { skillStore } from '$lib/stores/skills.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Sparkles } from '@lucide/svelte';

	const activeSkills = $derived(skillStore.activeSkills);
</script>

{#if activeSkills.length > 0}
	<div class="flex flex-wrap gap-1">
		{#each activeSkills as name}
			<Badge variant="outline" class="h-5 gap-1 px-1.5 text-[10px] border-[var(--cairn-accent)]/30 text-[var(--cairn-accent)]">
				<Sparkles class="h-2.5 w-2.5" />
				{name}
			</Badge>
		{/each}
	</div>
{/if}
