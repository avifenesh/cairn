import type { Rule, RuleExecution, SourceInfo, RuleTemplate } from '$lib/types';

let rules = $state<Rule[]>([]);
let executions = $state<RuleExecution[]>([]);
let sources = $state<SourceInfo[]>([]);
let templates = $state<RuleTemplate[]>([]);

export const ruleStore = {
	get rules() { return rules; },
	get executions() { return executions; },
	get sources() { return sources; },
	get templates() { return templates; },
	get enabledCount() { return rules.filter(r => r.enabled).length; },

	setRules(r: Rule[]) { rules = r; },
	setExecutions(e: RuleExecution[]) { executions = e; },
	setSources(s: SourceInfo[]) { sources = s; },
	setTemplates(t: RuleTemplate[]) { templates = t; },

	addRule(rule: Rule) {
		rules = [rule, ...rules];
	},
	updateRule(id: string, updates: Partial<Rule>) {
		rules = rules.map(r => r.id === id ? { ...r, ...updates } : r);
	},
	removeRule(id: string) {
		rules = rules.filter(r => r.id !== id);
	},
	addExecution(exec: RuleExecution) {
		executions = [exec, ...executions].slice(0, 100);
	},
};
