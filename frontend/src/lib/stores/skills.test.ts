import { describe, it, expect, beforeEach } from 'vitest';
import { skillStore } from './skills.svelte';

describe('skillStore', () => {
	beforeEach(() => {
		skillStore.setSkills([]);
		skillStore.setActiveSkills([]);
		skillStore.setSelectedSkill(null);
	});

	it('stores and retrieves skills', () => {
		const skills = [{ name: 'deploy', description: 'Deploy', scope: 'on-demand', inclusion: 'on-demand', disableModelInvocation: false, userInvocable: true }];
		skillStore.setSkills(skills);
		expect(skillStore.skills).toHaveLength(1);
		expect(skillStore.skills[0].name).toBe('deploy');
	});

	it('tracks active skills', () => {
		skillStore.activateSkill('web-search');
		expect(skillStore.activeSkills).toContain('web-search');
		expect(skillStore.isActive('web-search')).toBe(true);
	});

	it('deactivates skill', () => {
		skillStore.activateSkill('web-search');
		skillStore.deactivateSkill('web-search');
		expect(skillStore.isActive('web-search')).toBe(false);
	});

	it('does not add duplicate active skills', () => {
		skillStore.activateSkill('deploy');
		skillStore.activateSkill('deploy');
		expect(skillStore.activeSkills.filter((n) => n === 'deploy')).toHaveLength(1);
	});

	it('stores selected skill', () => {
		const skill = { name: 'test', description: 'Test', scope: 'always', inclusion: 'always', disableModelInvocation: false, userInvocable: false, content: '# Test' };
		skillStore.setSelectedSkill(skill);
		expect(skillStore.selectedSkill?.name).toBe('test');
		expect(skillStore.selectedSkill?.content).toBe('# Test');
	});
});
