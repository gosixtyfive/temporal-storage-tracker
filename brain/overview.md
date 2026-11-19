# Overview

## Purpose

temporal-storage-tracker is a **demo application** that shows how
[Temporal](https://temporal.io) workflows can drive a real-world, long-running
business process. The domain is deliberately simple and familiar so the
workflow concepts stay in focus.

## The scenario

A makerspace has a shared storage area. Members may leave items there
(projects in progress, materials, tools) for a limited time. The application
tracks each stored item and reminds the member as the storage time runs out
and after it has expired.

Each stored item is a long-running process with time-based steps, which is a
natural fit for a Temporal workflow.

## Roles

There is **no authentication** in the demo. A single person using the app can
switch between two roles:

| Role | What they do |
|------|--------------|
| **Member** | Stores an item and checks on its status |
| **Admin** | Watches every stored item across the space |

## Core features

1. **Store an item.** A simple form where a member registers an item they
   are leaving in the shared area.
2. **Item status view.** Shows one stored item: its details, how much
   storage time is left, and where it is in its lifecycle.
3. **Dashboard view.** An admin overview of all stored items and their
   states, highlighting those that are expiring soon or have expired.
4. **Notifications.** The member is notified when:
   - the item's storage time is **about to expire**,
   - the item **has expired** (once; nothing further is sent after expiry),
   - an admin **removes** the item.

   Notifications are shown in the UI (on the item status view and the
   dashboard). No email or other external delivery.

## Storage timing

- The storage duration is **fixed** for all items.
- The "expiring soon" reminder fires at a fixed portion of that duration
  before the end.
- Real-world reference: a **7-day** duration with the reminder **1 day
  before** expiry.
- **Demo time compression.** Timings must be compressible (for example,
  minutes instead of days) so a full lifecycle can be shown live. The
  reminder keeps the same proportion of the duration (1/7 before expiry)
  at any scale.

## Item lifecycle

```
Stored ──▶ Expiring soon ──▶ Expired
   │              │              │
   ├──────────────┼──────────────┼──▶ Collected by member
   └──────────────┴──────────────┴──▶ Removed by admin
```

- **Stored**: in the storage area, within its allowed time.
- **Expiring soon**: in the final part of its time; the member has been
  reminded.
- **Expired**: past its allowed time; the member has been notified once.
- **Collected by member**: the member took the item back. Ends tracking.
- **Removed by admin**: an admin cleared the item from the storage area.
  This can happen **at any time**, not only after expiry (typically because
  the member did not collect an expired item). The member is notified.
  Ends tracking.

Storage time cannot be extended.

Collected and Removed are separate end states. The dashboard can then show
how often items are collected on time and how often an admin has to step in.

## Non-goals (for the demo)

- User accounts, authentication, or real permissions
- Production-grade notification delivery
- Physical storage management (locations, capacity)

## Open questions

_None right now._

## Resolved

- Notifications are shown in the UI only; no email.
- Storage time cannot be extended.
- An admin can remove an item at any time, and the member is notified.

- Storage duration is fixed (real world: 7 days).
- The "expiring soon" reminder fires at a fixed portion before expiry (real
  world: 1 day before).
- Exactly one notification is sent at expiry, with nothing after it.
- Timings are compressible for demos.
- Admin removal of an uncollected item is a separate end state from member
  collection.
