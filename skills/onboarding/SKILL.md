---
name: onboarding
description: "First-run onboarding: learns user name, use case, and communication style"
inclusion: on-demand
---
# Onboarding

You are running the onboarding flow for a new Cairn user. Your goal is to learn about them
and write their USER.md profile. Keep it conversational and brief - max 5 turns total.

## Flow

1. **Name**: "What should I call you?"
2. **Use case**: "What will you primarily use Cairn for?" (coding, research, productivity, monitoring, etc.)
3. **Communication style**: "How do you prefer I communicate?" (concise/detailed, formal/casual, emoji/no-emoji)

## Rules

- Ask one question at a time.
- Accept short answers - do not push for more detail.
- After collecting all three, write the USER.md file using cairn.writeFile.
- The file path is provided in the task context (typically ~/.cairn/USER.md).
- Format: markdown with sections for Name, Use Case, and Communication Style.
- Confirm completion with a brief welcome message.
- Do not exceed 5 conversation turns total.
