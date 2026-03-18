// Skills store — skill catalog, active skills, detail loading

import type { Skill } from '$lib/types';

let skills = $state<Skill[]>([]);
let activeSkills = $state<string[]>([]);
let loading = $state(false);
let selectedSkill = $state<(Skill & { content?: string }) | null>(null);

export const skillStore = {
	get skills() { return skills; },
	get activeSkills() { return activeSkills; },
	get loading() { return loading; },
	get selectedSkill() { return selectedSkill; },

	setSkills(s: Skill[]) { skills = s; },
	setActiveSkills(names: string[]) { activeSkills = names; },
	setLoading(v: boolean) { loading = v; },
	setSelectedSkill(s: (Skill & { content?: string }) | null) { selectedSkill = s; },

	activateSkill(name: string) {
		if (!activeSkills.includes(name)) {
			activeSkills = [...activeSkills, name];
		}
	},

	deactivateSkill(name: string) {
		activeSkills = activeSkills.filter((n) => n !== name);
	},

	isActive(name: string): boolean {
		return activeSkills.includes(name);
	},
};
