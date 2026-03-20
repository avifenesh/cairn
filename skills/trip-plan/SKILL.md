---
name: trip-plan
description: "Plan a trip with day-by-day itinerary, activities, restaurants, and practical tips. Keywords: trip, travel, vacation, itinerary, plan trip, weekend in, visit, holiday, getaway, destination"
argument-hint: "[destination and dates]"
---

# Trip Planner

Quick travel itinerary. For a full artifact with packing list and citations, the TripRunner handles it via "plan a trip to X" intent.

## Steps

1. **Clarify essentials** (ask if not provided)
   - Destination
   - Dates / duration
   - Budget level (budget / mid-range / luxury)
   - Interests (food, culture, nature, nightlife, history)
   - Travel party (solo, couple, family, group)

2. **Research destination** via web search
   ```
   cairn.shell: curl -s 'http://127.0.0.1:8888/search?q=DESTINATION+travel+guide+2026&format=json' | jq '.results[:5] | .[] | {title, url, content}'
   cairn.shell: curl -s 'http://127.0.0.1:8888/search?q=best+restaurants+DESTINATION&format=json' | jq '.results[:3] | .[] | {title, url, content}'
   cairn.shell: curl -s 'http://127.0.0.1:8888/search?q=top+things+to+do+DESTINATION&format=json' | jq '.results[:3] | .[] | {title, url, content}'
   ```

3. **Check weather** (if dates known)
   ```
   cairn.shell: curl -s 'http://127.0.0.1:8888/search?q=DESTINATION+weather+MONTH+2026&format=json' | jq '.results[0].content'
   ```

4. **Build itinerary**

```markdown
## Trip to [Destination] — [Dates]

### Day 1: Arrival + [Theme]
**Morning**: Arrive, check in, settle
**Afternoon**: [Activity] — [why it's good, practical tips]
**Evening**: Dinner at [Restaurant] — [cuisine, price range]

### Day 2: [Theme]
**Morning**: [Activity]
**Afternoon**: [Activity]
**Evening**: [Activity or restaurant]

### Day N: Departure
**Morning**: [Last activity or breakfast spot]
**Departure**: [Transport tips]

### Practical Info
- **Getting around**: [transport options, costs]
- **Budget estimate**: [daily cost breakdown]
- **Tips**: [local customs, tipping, safety]
- **Book ahead**: [things that need reservations]
```

5. **Offer follow-ups**: "Want a packing list?" / "Check calendar for conflicts?" / "Create full trip artifact?"

## Notes

- Include 1 backup activity per day (rain plan)
- Balance busy days with downtime
- Mention walking distances between activities
- For budget trips, include free activities
