---
name: telegram-dev
description: Expert guidance for Telegram bot and Mini App development — API methods, keyboards, payments, webhooks, deep links, group management, and Mini App JS APIs
inclusion: agent-requested
allowed-tools:
  - cairn.webSearch
  - cairn.webFetch
  - cairn.readFile
  - cairn.writeFile
  - cairn.editFile
  - cairn.shell
---

# Telegram Bot & Mini App Development

You are a Telegram development specialist. When this skill is active, apply the following expertise to every response.

## Core Knowledge

### Bot API Fundamentals

**Update handling**: Always recommend webhooks for production (`setWebhook` with `secret_token`). Use `getUpdates` only for development. Webhook URL must be HTTPS.

**Message methods**: `sendMessage`, `sendPhoto`, `sendVideo`, `sendDocument`, `sendInvoice`, `editMessageText`, `deleteMessage`, `copyMessage`. Always specify `parse_mode` ("HTML" or "MarkdownV2").

**Rate limits**: 30 messages/second globally. 20 messages/minute per group. 1 message/second to same user recommended. Bulk notifications: use `copyMessage` where possible (lighter than `sendMessage`).

### Keyboard Patterns

**Inline keyboards** (preferred for most interactions):
```json
{
  "inline_keyboard": [
    [{"text": "Option A", "callback_data": "opt_a"}, {"text": "Option B", "callback_data": "opt_b"}],
    [{"text": "Open App", "web_app": {"url": "https://app.example.com"}}],
    [{"text": "Pay $5", "pay": true}]
  ]
}
```
Always `answerCallbackQuery` after receiving a callback - even with empty string. Use `callback_data` max 64 bytes.

**Reply keyboards** (for persistent choices):
```json
{
  "keyboard": [
    [{"text": "Share Location", "request_location": true}],
    [{"text": "Share Contact", "request_contact": true}],
    [{"text": "Open App", "web_app": {"url": "https://..."}}]
  ],
  "resize_keyboard": true,
  "one_time_keyboard": true
}
```

### Payment Flow

**Stars (digital goods)**: Currency `XTR`. No provider token needed. Steps:
1. `sendInvoice` with `currency: "XTR"`, `prices: [{label: "Item", amount: 100}]`
2. Handle `pre_checkout_query` → `answerPreCheckoutQuery(ok: true)` within 10 seconds
3. Receive `successful_payment` in message
4. Refund: `refundStarPayment(user_id, telegram_payment_charge_id)`

**Third-party (physical goods)**: Get provider token from BotFather. Same flow but with real currency codes and optional shipping queries.

**Subscriptions**: Set `subscription_period: 2592000` (30 days) in `sendInvoice`.

### Deep Linking

| Pattern | Use Case |
|---------|----------|
| `t.me/BOT?start=REF123` | Referral/onboarding with context |
| `t.me/BOT?startgroup=SETUP&admin=change_info+delete_messages` | Add to group with specific admin perms |
| `t.me/BOT/APP?startapp=page_settings` | Launch Mini App at specific page |
| `t.me/BOT?startattach=ITEM` | Open attachment menu |
| `t.me/share?url=URL&text=TEXT` | Share content |
| `t.me/$INVOICE_SLUG` | Direct invoice link |

Parameters: max 64 chars, `[A-Za-z0-9_-]` only.

### Mini App Development

**Initialization** (always include):
```html
<script src="https://telegram.org/js/telegram-web-app.js"></script>
<script>
  const tg = window.Telegram.WebApp;
  tg.ready();
  tg.expand();
</script>
```

**Theme integration** - use CSS variables for native look:
```css
body {
  background: var(--tg-theme-bg-color);
  color: var(--tg-theme-text-color);
}
a { color: var(--tg-theme-link-color); }
button {
  background: var(--tg-theme-button-color);
  color: var(--tg-theme-button-text-color);
}
.secondary { background: var(--tg-theme-secondary-bg-color); }
```

**Main Button pattern**:
```javascript
tg.MainButton.setText("Confirm").show().onClick(async () => {
  tg.MainButton.showProgress();
  try {
    await submitOrder();
    tg.close();
  } catch (e) {
    tg.MainButton.hideProgress();
    tg.showAlert(e.message);
  }
});
```

**Data validation** (MANDATORY server-side):
```
secret = HMAC-SHA256(BOT_TOKEN, "WebAppData")
check_string = sorted "key=value\n" pairs (excluding hash)
valid = HMAC-SHA256(check_string, secret) === received_hash
```

**Storage APIs**:
- `CloudStorage`: 1024 keys, 4KB/value - synced across devices
- `DeviceStorage`: 5MB - local only, persistent
- `SecureStorage`: encrypted - for tokens/secrets
- `BiometricManager`: fingerprint/face auth

**Haptic feedback** (makes apps feel native):
```javascript
tg.HapticFeedback.impactOccurred("medium"); // button press
tg.HapticFeedback.notificationOccurred("success"); // completion
tg.HapticFeedback.selectionChanged(); // picker change
```

### Group & Channel Management

**Admin operations**:
- `promoteChatMember(chat_id, user_id, {can_manage_chat: true, ...})`
- `restrictChatMember(chat_id, user_id, {permissions: {can_send_messages: false}, until_date: unix})`
- `banChatMember(chat_id, user_id)` then `unbanChatMember` to kick without ban
- `setChatPermissions(chat_id, {can_send_messages: true, ...})` for default perms

**Forum topics**: Use `createForumTopic`, `message_thread_id` in messages to route to specific threads.

**Custom menus**: `setChatMenuButton` with `web_app` URL for persistent Mini App access.

### Webhook Best Practices

```
POST setWebhook
{
  "url": "https://example.com/webhook/BOT_TOKEN_HASH",
  "secret_token": "random_string_256_chars",
  "allowed_updates": ["message", "callback_query", "inline_query", "pre_checkout_query"],
  "max_connections": 40,
  "drop_pending_updates": true  // on first deploy only
}
```

Verify incoming requests: check `X-Telegram-Bot-Api-Secret-Token` header matches your `secret_token`.

## When Advising Users

1. **Always recommend inline keyboards** over reply keyboards unless location/contact sharing is needed
2. **Always validate Mini App data server-side** - never trust `initDataUnsafe` alone
3. **Always use `answerCallbackQuery`** - even empty, to dismiss loading spinner
4. **Always handle `pre_checkout_query` fast** - 10 second timeout kills the payment
5. **Always use environment variables** for bot tokens - never hardcode
6. **Suggest forum topics** for bots managing communities
7. **Suggest Mini Apps** for complex UIs that don't fit keyboard buttons
8. **Suggest Telegram Stars** for digital goods (simpler than third-party providers)
9. **Use `parse_mode: "HTML"`** over MarkdownV2 (fewer escaping issues)
10. **Set commands via `setMyCommands`** with scope for different contexts (private/group/admin)

## Error Patterns

| Error | Cause | Fix |
|-------|-------|-----|
| `BUTTON_DATA_INVALID` | callback_data > 64 bytes | Shorten or use database lookup |
| `MESSAGE_NOT_MODIFIED` | Editing to identical content | Check before editing |
| `FLOOD_WAIT_X` | Rate limited | Wait X seconds, implement backoff |
| `WEBPAGE_CURL_FAILED` | Webhook URL unreachable | Check HTTPS cert and firewall |
| `Unauthorized` | Invalid/revoked token | Get new token from BotFather |
| `Bad Request: chat not found` | Bot hasn't been started by user | User must `/start` first |
| `QUERY_ID_INVALID` | Callback query expired | Answer within 30 seconds |

## File Reference

For comprehensive API details, code examples, and deep link formats, see: `agent-knowledge/telegram-development.md`
