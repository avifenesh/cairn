<script lang="ts">
	import { X } from '@lucide/svelte';

	let { open = false, onclose }: { open: boolean; onclose: () => void } = $props();

	const shortcuts = [
		{ key: '1-8', action: 'Switch view' },
		{ key: 'j / k', action: 'Navigate items' },
		{ key: 'o', action: 'Open item URL' },
		{ key: 'r', action: 'Mark read' },
		{ key: 's', action: 'Manual sync' },
		{ key: 'a', action: 'Approve (ops)' },
		{ key: 'd', action: 'Deny (ops)' },
		{ key: 't', action: 'Toggle theme' },
		{ key: '?', action: 'This help' },
		{ key: 'Esc', action: 'Close modals' },
	];
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onclick={onclose}
		onkeydown={(e) => e.key === 'Escape' && onclose()}
	>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="w-full max-w-sm rounded-xl border border-border-subtle bg-[var(--bg-1)] p-5 shadow-lg"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.key === 'Escape' && onclose()}
		>
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-sm font-semibold text-[var(--text-primary)]">Keyboard Shortcuts</h2>
				<button
					class="rounded p-1 hover:bg-[var(--bg-3)] transition-colors"
					onclick={onclose}
					aria-label="Close"
				>
					<X class="h-4 w-4 text-[var(--text-tertiary)]" />
				</button>
			</div>
			<div class="flex flex-col gap-1.5">
				{#each shortcuts as s}
					<div class="flex items-center justify-between py-1">
						<span class="text-xs text-[var(--text-secondary)]">{s.action}</span>
						<kbd class="rounded border border-border-subtle bg-[var(--bg-3)] px-1.5 py-0.5 text-[10px] font-mono text-[var(--text-tertiary)]">
							{s.key}
						</kbd>
					</div>
				{/each}
			</div>
			<div class="mt-4 border-t border-border-subtle pt-3">
				<div class="flex items-center justify-between py-1">
					<span class="text-xs text-[var(--text-secondary)]">Command palette</span>
					<kbd class="rounded border border-border-subtle bg-[var(--bg-3)] px-1.5 py-0.5 text-[10px] font-mono text-[var(--text-tertiary)]">
						Cmd+K
					</kbd>
				</div>
			</div>
		</div>
	</div>
{/if}
