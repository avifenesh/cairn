import type { Rule, RuleExecution } from '$lib/types';

let rules = $state<Rule[]>([]);
let executions = $state<RuleExecution[]>([]);
let loading = $state(false);

export const ruleStore = {
	get rules() { return rules; },
	get executions() { return executions; },
	get loading() { return loading; },
	get enabledCount() { return rules.filter(r => r.enabled).length; },

	setRules(r: Rule[]) { rules = r; },
	setExecutions(e: RuleExecution[]) { executions = e; },
	setLoading(v: boolean) { loading = v; },

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
