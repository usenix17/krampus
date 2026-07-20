import apiClient from './client';

// createRule creates (or idempotently updates) a rule. Used by the one-click
// allowlist actions on the Programs and Events pages.
export const createRule = ({ identifier, rule_type, policy = 'ALLOWLIST', comment = '', custom_message = '' }) => {
  return apiClient.post('/api/rules', { identifier, rule_type, policy, comment, custom_message });
};
