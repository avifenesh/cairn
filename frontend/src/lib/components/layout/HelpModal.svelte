<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import { Separator } from '$lib/components/ui/separator';

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

<Dialog.Root {open} onOpenChange={(v) => !v && onclose()}>
	<Dialog.Content class="max-w-sm">
		<Dialog.Header>
			<Dialog.Title>Keyboard Shortcuts</Dialog.Title>
		</Dialog.Header>
		<div class="flex flex-col gap-1.5">
			{#each shortcuts as s}
				<div class="flex items-center justify-between py-1">
					<span class="text-xs text-[var(--text-secondary)]">{s.action}</span>
					<kbd class="rounded border border-border-subtle bg-[var(--bg-2)] px-1.5 py-0.5 text-[10px] font-mono text-[var(--text-tertiary)]">
						{s.key}
					</kbd>
				</div>
			{/each}
		</div>
		<Separator class="my-2" />
		<div class="flex items-center justify-between py-1">
			<span class="text-xs text-[var(--text-secondary)]">Command palette</span>
			<kbd class="rounded border border-border-subtle bg-[var(--bg-2)] px-1.5 py-0.5 text-[10px] font-mono text-[var(--text-tertiary)]">
				⌘K
			</kbd>
		</div>
	</Dialog.Content>
</Dialog.Root>
