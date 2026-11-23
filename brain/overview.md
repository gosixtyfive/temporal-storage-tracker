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

## People in the scenario

**Members** leave items in the storage area and collect them again.
**Admins** look after the storage area and can remove items. These are
people in the scenario, not roles in the app: the demo has **no accounts and
no role switching**. Anyone using it can add items and collect or remove
them.

## Core features

The app has two views, with a single switch between them:

1. **Add item.** A simple form to register an item a member is leaving in
   the shared area. Each item added starts its own workflow. The form stays
   open so several items can be added in a row.
2. **Dashboard.** Every stored item with its state, how much storage time is
   left, and its notifications, highlighting items that are expiring soon or
   have expired. From here an item can be **collected** (by its member) or
   **removed** (by an admin).
3. **Notifications.** The member is notified when:
   - the item's storage time is **about to expire**,
   - the item **has expired** (once; nothing further is sent after expiry),
   - an admin **removes** the item.

   Notifications are shown on the dashboard. No email or other external
   delivery.

## Running a demo

The intended setup is **two browser windows side by side**: one on Add item
to enter items, one on the Dashboard to watch every item move through its
lifecycle as it happens. The Temporal Web UI can sit alongside to show the
workflows themselves.

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

- The app has two views, Add item and Dashboard, and no roles. Collect and
  remove both happen on the dashboard; there is no separate item status
  page. (Replaced an earlier Member/Admin role switch with a status view.)
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
