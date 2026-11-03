# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

Marketing site in `aquifyFrontend/`: static HTML/CSS/JS (user choice), separate from the Next.js `admin/` app.

## Users

Swim team admins (club managers, registrars, treasurers, often volunteers) and swim team coaches. Not tech-savvy: they run a club on spreadsheets, email, and legacy tools like TeamUnify/SportsEngine Motion. Visitors are evaluating a replacement; they need plain language, not software jargon. Parents and swimmers use portals but are not the target of this site.

## Product Purpose

aquify is a swim club operations platform: the team's system of record for roster and households, meets, money, messaging, and training. Success for the site: a coach or admin understands in seconds what it does for them and joins the waitlist.

## Positioning

Leads with giving coaches their time back (practice and workout planning, attendance, pool-deck tools), with admin work (registration, billing, meets) as the second beat. Meet files (CL2/SD3/HY3) import and export natively, so meet day stops being a manual chore.

## Operating Context

Pool deck, clipboards, phones, meet-entry files sent to hosts, parent email threads, season registration, invoices, volunteer shift sheets. Coaches work on a phone at the pool; admins work at a desk.

## Capabilities and Constraints

Confirmed (from the codebase): households and registrations; meet commitments, entries, coach approval, SD3 export, CL2/SD3/HY3 results import, best times and team records; structured workouts, attendance, training analytics, deck view; billing with invoices, fee schedules, sibling discounts, autopay, Stripe; announcements and email campaigns; volunteer jobs and service hours; club events with calendar feeds; wellness check-ins; parent and swimmer portals; public team website; installable PWA.
Primary CTA: join the waitlist. Pricing, launch date, and customers are undecided: do not invent.
Copy rule: plain language for non-technical readers; no jargon (no "system of record", "interoperability", "SDIF" without a plain explanation).

## Brand Commitments

Name: aquify. Existing in-app accent `#eb5d1e` and design guide `design-language-guide.html` belong to the product app; the marketing site's visual world is open.

## Evidence on Hand

Real product (screenshots can be produced from `admin/`). No customers, testimonials, pricing, benchmarks, or stats exist: do not fabricate. Demo data must be labeled synthetic.

## Product Principles

1. Speak coach and club-admin language; explain by the task, not the technology.
2. Lead with coach time saved, then admin relief.
3. Show the product doing the job; never invent claims.
4. Make the waitlist ask low-friction.

## Accessibility & Inclusion

WCAG AA minimum; readable at pool-deck distance on a phone; reduced-motion respected.
