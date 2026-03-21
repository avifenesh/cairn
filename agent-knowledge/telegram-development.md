# Learning Guide: Telegram Bot API & Mini Apps Development

**Generated**: 2026-03-21
**Sources**: 6 official Telegram documentation pages analyzed
**Depth**: deep

## Prerequisites

- HTTP/HTTPS and webhook concepts
- JSON parsing
- Basic JavaScript (for Mini Apps)
- A Telegram account and @BotFather access

## TL;DR

- Bots are created via @BotFather, which gives you an API token
- Two update modes: polling (`getUpdates`) or webhooks (`setWebhook`) - webhooks for production
- Inline keyboards are the primary interaction pattern (callback buttons on messages)
- Mini Apps (Web Apps) are full JavaScript apps running inside Telegram with native-like APIs
- Payments support both Telegram Stars (digital goods) and third-party providers (physical goods)
- Deep links enable rich navigation: `t.me/bot?start=param`, `t.me/bot/app?startapp=param`
- Bot API supports 160+ currencies, forum topics, business connections, paid media, and stories

## Core Concepts

### Bot Creation & Setup

Create via @BotFather with `/newbot`. The token format is `123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11`. Treat it like a password. Configure commands with `/setcommands`, enable inline mode with `/setinline`, set up payments via Bot Settings > Payments.

### Update Handling

**Webhooks (production)**: `setWebhook` with HTTPS URL. Telegram POSTs JSON updates. Set `secret_token` header for verification. Max 100 concurrent connections.

**Long polling (development)**: `getUpdates` with `offset` parameter. Updates stored 24h server-side.

### Message Types & Sending

| Method | Purpose |
|--------|---------|
| `sendMessage` | Text with Markdown/HTML formatting |
| `sendPhoto/Video/Audio/Document` | Media with captions |
| `sendPoll` | Interactive polls |
| `sendLocation` | Live/static location |
| `sendInvoice` | Payment requests |
| `editMessageText` | Modify sent messages |
| `deleteMessage` | Remove messages |
| `copyMessage` | Duplicate without forwarding header |

### Keyboards

**Reply Keyboard** (`ReplyKeyboardMarkup`): Replaces user's keyboard. Good for simple choices. Can request location, contact, or launch web apps.

**Inline Keyboard** (`InlineKeyboardMarkup`): Buttons attached to messages. Each button has:
- `callback_data` - triggers `CallbackQuery` to bot
- `url` - opens a link
- `web_app` - launches a Mini App
- `login_url` - seamless website auth
- `pay` - payment button (must be first button)
- `switch_inline_query` - switch to inline mode

Always `answerCallbackQuery` to dismiss the loading indicator.

### Inline Mode

Users type `@botname query` in any chat. Bot returns results via `answerInlineQuery`. Enable via BotFather. Great for search bots, GIF bots, content sharing.

### Deep Linking

| Format | Use |
|--------|-----|
| `t.me/bot?start=PARAM` | Private chat with start parameter |
| `t.me/bot?startgroup=PARAM&admin=PERMS` | Add bot to group with admin rights |
| `t.me/bot/app?startapp=PARAM` | Launch Mini App directly |
| `t.me/bot?startattach=PARAM` | Open attachment menu |
| `t.me/share?url=URL&text=TEXT` | Share content |
| `t.me/$SLUG` or `t.me/invoice/SLUG` | Payment invoice |

Parameters: up to 64 chars, alphanumeric + `_` + `-`.

## Mini Apps (Web Apps)

### Architecture

Mini Apps are HTML/JS/CSS apps running in Telegram's WebView. They get native access to:
- Theme colors (auto-sync with Telegram theme)
- Haptic feedback
- Cloud storage (1024 keys, 4KB per value)
- Device storage (5MB per bot)
- Secure storage (Keychain/Keystore encrypted)
- Biometric auth
- Device sensors (accelerometer, gyroscope, orientation)
- Location services
- Main/Secondary action buttons

### Initialization

```html
<script src="https://telegram.org/js/telegram-web-app.js"></script>
<script>
  const tg = window.Telegram.WebApp;
  tg.ready(); // Signal UI is ready
  tg.expand(); // Full height

  // Access user data (validate server-side!)
  const user = tg.initDataUnsafe.user;

  // Theme-aware styling via CSS vars
  // var(--tg-theme-bg-color), var(--tg-theme-text-color), etc.
</script>
```

### Main Button Pattern

```javascript
tg.MainButton
  .setText("Submit Order")
  .show()
  .onClick(() => {
    tg.MainButton.showProgress();
    // Send data to your server
    fetch('/api/order', { method: 'POST', body: JSON.stringify(data) })
      .then(() => tg.close());
  });
```

### Data Validation (Server-Side)

```
secret_key = HMAC-SHA256(bot_token, "WebAppData")
data_check_string = alphabetically sorted "key=value" pairs joined with "\n"
computed_hash = HMAC-SHA256(data_check_string, secret_key)
// Compare computed_hash with received hash
```

### Key Events

| Event | When |
|-------|------|
| `themeChanged` | User switches theme |
| `viewportChanged` | Resize (use `viewportStableHeight` for fixed elements) |
| `mainButtonClicked` | Main button tapped |
| `backButtonClicked` | Back button tapped |
| `invoiceClosed` | Payment flow completed |
| `fullscreenChanged` | Fullscreen toggled |

### Launch Methods

1. **Keyboard button**: `KeyboardButton` with `web_app` URL
2. **Inline button**: `InlineKeyboardButton` with `web_app` URL
3. **Menu button**: Set via `setChatMenuButton`
4. **Direct link**: `t.me/bot/appname?startapp=param`
5. **Inline mode**: Return Mini App results
6. **Attachment menu**: Configure via BotFather

## Payments

### Telegram Stars (Digital Goods)

Use currency code `XTR`. No third-party provider needed. Flow:
1. `sendInvoice` with `XTR` currency
2. User pays with Stars
3. `pre_checkout_query` - bot validates within 10 seconds
4. `successful_payment` message received
5. Refund via `refundStarPayment`

### Third-Party Providers (Physical Goods)

1. Set up provider via BotFather (Stripe, etc.)
2. `sendInvoice` with provider token and currency
3. Optional: collect shipping info via `answerShippingQuery`
4. `answerPreCheckoutQuery` within 10 seconds
5. `successful_payment` confirmation

Telegram charges 0% commission. Provider fees apply (e.g., Stripe 2.9% + $0.30).

### Subscription Model

Use `subscription_period` in `sendInvoice` for recurring payments. Stars subscriptions auto-renew.

## Group & Channel Management

| Method | Purpose |
|--------|---------|
| `promoteChatMember` | Grant admin rights with specific permissions |
| `restrictChatMember` | Set user restrictions (send messages, media, etc.) |
| `banChatMember` | Ban user from group |
| `setChatPermissions` | Default permissions for all members |
| `setChatTitle/Description/Photo` | Modify group metadata |
| `createForumTopic` | Create forum thread |
| `pinChatMessage` | Pin important messages |
| `getChatMemberCount` | Get member count |

Admin permissions configurable: `change_info`, `post_messages`, `edit_messages`, `delete_messages`, `invite_users`, `restrict_members`, `pin_messages`, `manage_topics`, `promote_members`.

## Business Features

- **Business Connections**: Bots manage customer conversations for business accounts
- **Paid Media**: Photos/videos behind Star paywall (up to 25,000 Stars)
- **Ad Revenue Sharing**: 50% from Telegram Ads in bot channels
- **Stars monetization**: In-app purchases, subscriptions, tips

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|---------------|--------------|
| Token exposure | Hardcoded in source | Use environment variables |
| Missing `answerCallbackQuery` | Forgot to acknowledge | Always answer, even with empty response |
| Pre-checkout timeout | Processing takes >10s | Validate quickly, process async |
| Mini App data trust | Using `initDataUnsafe` without validation | Always validate `initData` server-side |
| Webhook certificate issues | Self-signed cert not uploaded | Use `certificate` param in `setWebhook` or Let's Encrypt |
| Rate limits | Too many requests | Respect 30 msg/sec to same chat, 20 msg/min to same group |
| File size limits | Photos >10MB, files >50MB | Compress or use `url` upload for files up to 2GB |

## Best Practices

1. **Always use webhooks in production** - lower latency, no polling overhead
2. **Validate Mini App data server-side** - never trust `initDataUnsafe` alone
3. **Use inline keyboards over reply keyboards** - better UX, persists with message
4. **Implement `/start` with deep link params** - enables referrals and contextual onboarding
5. **Respect rate limits** - 30 messages/second globally, 20/minute per group
6. **Use `parse_mode: "HTML"` or `"MarkdownV2"`** - rich text formatting
7. **Handle errors gracefully** - Telegram returns descriptive error messages
8. **Use `chat_id` from updates** - never assume chat IDs
9. **Set bot commands via `setMyCommands`** - users see suggestions when typing /
10. **Use `message_thread_id` for forum topics** - route messages to correct threads

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| [Bot API Reference](https://core.telegram.org/bots/api) | Official Docs | Complete method reference |
| [Mini Apps Docs](https://core.telegram.org/bots/webapps) | Official Docs | Full WebApp JS API |
| [Bot Features](https://core.telegram.org/bots/features) | Official Docs | Capability overview |
| [Payments API](https://core.telegram.org/bots/payments) | Official Docs | Payment flow guide |
| [Deep Links](https://core.telegram.org/api/links) | Official Docs | All URL formats |
| [Bot Tutorial](https://core.telegram.org/bots/tutorial) | Official Tutorial | Step-by-step setup |

---

*Generated by /learn from 6 official Telegram documentation sources.*
*See `resources/telegram-development-sources.json` for full source metadata.*
