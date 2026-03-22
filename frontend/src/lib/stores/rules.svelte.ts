import type { Rule, RuleExecution } from '$lib/types';

let rules = $state<Rule[]>([]);
let executions = $state<RuleExecution[]>([]);

export const ruleStore = {
	get rules() { return rules; },
	get executions() { return executions; },
	get enabledCount() { return rules.filter(r => r.enabled).length; },

	setRules(r: Rule[]) { rules = r; },
	setExecutions(e: RuleExecution[]) { executions = e; },

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
