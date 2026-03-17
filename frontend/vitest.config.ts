import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	plugins: [svelte()],
	resolve: {
		conditions: ['browser'],
		alias: {
			$lib: new URL('./src/lib', import.meta.url).pathname,
			'$app/navigation': new URL('./src/test/mocks/navigation.ts', import.meta.url).pathname,
			'$app/state': new URL('./src/test/mocks/state.ts', import.meta.url).pathname,
		},
	},
	test: {
		environment: 'jsdom',
		include: ['src/**/*.test.ts'],
		globals: true,
		setupFiles: ['src/test/setup.ts'],
	},
});
