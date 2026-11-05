#!/usr/bin/env python3
"""Generates every static page in aquifyFrontend/. Run from aquifyFrontend/:  python3 tools/site.py"""
import pathlib
OUT = pathlib.Path(__file__).resolve().parent.parent

FEATURES = [
    ("coaches.html", "Coaches", "Workouts, roll call and the pool-deck view"),
    ("meets.html", "Meets", "Commitments, entries, results and records"),
    ("billing.html", "Billing", "Dues, discounts, autopay and invoices"),
    ("registration.html", "Registration", "Sign-ups, waivers, groups and rosters"),
    ("communication.html", "Communication", "Announcements, email and messages"),
    ("volunteers.html", "Volunteers and events", "Shifts, hours, RSVPs and fees"),
    ("performance.html", "Performance", "Best times, cuts, records and goals"),
    ("families.html", "Families", "Parent and swimmer portals"),
    ("website.html", "Team website", "A public page and a phone app"),
]
TOP = [("season.html", "A season"), ("switch.html", "Switching"), ("safety.html", "Safety"), ("faq.html", "FAQ")]
WATER = '<canvas id="water" aria-hidden="true"></canvas><div class="fallback"></div>'


def shell(file, title, desc, body):
    feat_here = any(file == f for f, _, _ in FEATURES)
    menu = "".join(f'<a href="{f}"{" aria-current=\"page\"" if f == file else ""}><b>{n}</b><span>{d}</span></a>' for f, n, d in FEATURES)
    top = "".join(f'<a href="{f}"{" aria-current=\"page\"" if f == file else ""}>{n}</a>' for f, n in TOP)
    cur = ' aria-current="page"' if file == "waitlist.html" else ""
    names = {f: n for f, n, _ in FEATURES}
    L = lambda *fs: "".join(f'<a href="{f}">{names[f]}</a>' for f in fs)
    foot_f = "".join(f'<a href="{f}">{n}</a>' for f, n, _ in FEATURES)
    foot_t = "".join(f'<a href="{f}">{n}</a>' for f, n in TOP)
    return f'''<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{title}</title>
<meta name="description" content="{desc}">
<meta name="theme-color" content="#04080b">
<link rel="icon" href="assets/icon.svg" type="image/svg+xml">
<link rel="preload" href="assets/fonts/inter-tight-latin-500-normal.woff2" as="font" type="font/woff2" crossorigin>
<link rel="stylesheet" href="assets/styles.css">
</head>
<body>
<header class="nav">
  <a class="brand" href="index.html" aria-label="aquify home">aquify<i>.</i></a>
  <button class="burger" aria-label="Menu" aria-expanded="false"><span></span><span></span></button>
  <nav class="links" aria-label="Main">
    <div class="dd"><button class="dd-btn" aria-expanded="false"{' style="color:var(--chalk)"' if feat_here else ''}>Features</button><div class="dd-menu">{menu}</div></div>
    {top}
    <a class="btn" href="waitlist.html"{cur}>Join the waitlist</a>
  </nav>
</header>
<main>
{body}
</main>
<footer class="foot">
  <div class="foot-grid">
    <div class="foot-lead">
      <p class="lede">Swim club software for coaches and admins.</p>
      <a class="btn" href="waitlist.html">Join the waitlist</a>
    </div>
    <nav aria-label="Footer">
      <div><b>Run the team</b>{L("coaches.html","meets.html","registration.html","communication.html")}</div>
      <div><b>More features</b>{L("billing.html","volunteers.html","performance.html","families.html","website.html")}</div>
      <div><b>Learn more</b>{foot_t}<a href="people.html">Who it’s for</a></div>
    </nav>
  </div>
  <div class="foot-mark" aria-hidden="true">aquify.</div>
  <small>Every screen on this site shows sample data for a made-up club. No real swimmers, families or payments.</small>
</footer>
<script src="assets/main.js" defer></script>
</body>
</html>
'''


# ---------- building blocks ----------
def win(src, alt, cls="win", extra=""):
    return f'<div class="{cls}" {extra}><img src="assets/img/{src}.jpg" alt="{alt}" loading="lazy" decoding="async"></div>'


def phone(src, alt, bg="#fcfbfa", bot="#f1e7d8", ind="rgba(0,0,0,.35)"):
    return f'<div class="phone"><div class="screen" style="--sb:{bg};--sbb:{bot};--ind:{ind}"><img src="assets/img/{src}.jpg" alt="{alt}" loading="lazy" decoding="async"></div></div>'


def hero(h1, lede, chips="", maxch="11ch"):
    return f'<section class="page-hero">{WATER}<div class="in"><h1 class="d1" style="max-width:{maxch}">{h1}</h1>{chips}<p class="lede">{lede}</p></div></section>'


def say(text, length="280vh"):
    return f'<section class="stage say" data-len="{length}"><div class="pin"><p>{text}</p></div></section>'


def rise(img, mobile, alt, calls):
    cs = "".join(f'<div class="call" data-r="{a},{b}">{h}<span>{s}</span></div>' for (h, s), (a, b) in zip(calls, [(.55, .68), (.66, .79), (.77, .9)]))
    return f'''
<section class="stage rise" data-len="300vh" aria-label="{alt}">
  <div class="pin">
    <div class="shot" data-r="0.02,0.55"><div class="win"><picture><source media="(max-width:760px)" srcset="assets/img/{mobile}.jpg"><img src="assets/img/{img}.jpg" alt="{alt}" decoding="async"></picture></div></div>
    <div class="calls">{cs}</div>
  </div>
</section>'''


def steps_stage(h2, steps, layers, length=None, numbered=False):
    length = length or f"{max(340, 150 * len(steps))}vh"
    st = "".join(f'<div class="step">{"<span class=n>%d</span>" % (i + 1) if numbered else ""}{h}<small>{s}</small></div>' for i, (h, s) in enumerate(steps))
    ly = "".join(win(i, a) for i, a in layers)
    return f'''
<section class="stage" data-len="{length}" data-steps="{len(steps)}">
  <div class="pin"><div class="split">
    <div class="txt"><h2 class="d2">{h2}</h2><div class="steps">{st}</div></div>
    <div class="layers">{ly}</div>
  </div></div>
</section>'''


def zoom_stage(img, alt, caption, ox="18%", oy="24%"):
    return f'''
<section class="stage" data-len="260vh" aria-label="{alt}">
  <div class="pin" style="display:grid;place-items:center">
    <div class="zoomwin" style="--ox:{ox};--oy:{oy}" data-r="0.08,0.85">{win(img, alt)}</div>
    <div class="cap" data-r="0.5,0.72"><h2 class="d3">{caption}</h2></div>
  </div>
</section>'''


def fly(phones, caption, lede, bg=("#fcfbfa", "#f1e7d8")):
    ph = "".join(f'<div class="phone" data-r="{a},{b}" style="--r:{r}deg"><div class="screen" style="--sb:{bg[0]};--sbb:{bg[1]}"><img src="assets/img/{i}.jpg" alt="{al}" loading="lazy" decoding="async"></div></div>' for (i, al), (a, b), r in zip(phones, [(.04, .4), (.18, .54), (.32, .68)], [-7, 0, 7]))
    return f'''
<section class="stage fly" data-len="320vh">
  <div class="pin"><div class="phones">{ph}</div>
    <div class="flycap" data-r="0.68,0.86"><h2 class="d3" style="max-width:15ch">{caption}</h2><p class="lede" style="max-width:34ch">{lede}</p></div>
  </div>
</section>'''


def feature(img, alt, h2, copy, facts=(), flip=False, par=0.04, phone_img=None):
    fl = "".join(f"<li>{f}</li>" for f in facts)
    fl = f'<ul class="facts">{fl}</ul>' if fl else ""
    vis = win(img, alt) if not phone_img else f'<div class="phones" style="justify-content:center">{phone(phone_img, alt)}</div>'
    cls = "row flip" if flip else "row"
    return f'''<section class="sect tight"><div class="wrap {cls}">
  <div data-par="{par}">{vis}</div>
  <div class="txt"><h2 class="d2">{h2}</h2><p class="copy">{copy}</p>{fl}</div>
</div></section>'''


def index(rows, links=False):
    if links:
        li = "".join(f'<li><a href="{h}"><b>{a}</b><span>{b}</span></a></li>' for h, a, b in rows)
        return f'<section class="sect tight"><div class="wrap"><ul class="index rl">{li}</ul></div></section>'
    li = "".join(f"<li><b>{a}</b><span>{b}</span></li>" for a, b in rows)
    return f'<section class="sect tight"><div class="wrap"><ul class="index">{li}</ul></div></section>'


def cta(h="Ready when your club is.", p="We’re opening aquify to clubs in order. Leave your email and we’ll tell you when it’s your turn."):
    return f'''
<section class="cta"><div class="in">
  <h2 class="d1" style="max-width:9ch">{h}</h2>
  <p class="lede">{p}</p>
  <a class="btn" href="waitlist.html" style="height:60px;padding:0 34px;font-size:18px">Join the waitlist</a>
</div></section>'''


SHORTHAND = '''
<section class="stage" data-shorthand data-len="420vh" aria-label="Shorthand becomes a structured workout">
  <div class="pin"><div class="split">
    <div class="txt">
      <h2 class="d2">Type it like you’d write it on the whiteboard.</h2>
      <p class="lede">aquify reads the sets, rest, zones and equipment, then adds up the yardage for you.</p>
    </div>
    <div class="sh">
      <div class="pane"><h4>What the coach types</h4><div class="typed" aria-hidden="true"></div></div>
      <div class="pane out"><h4>What the swimmers get</h4>
        <div class="sec"><b>Warmup</b><span>800 yd</span></div>
        <div class="sec"><b>Main set</b><span>1,500 yd</span></div>
        <div class="sec"><b>Cooldown</b><span>200 yd</span></div>
        <div class="tot"><b data-total>0</b><span>yards, <span data-lengths>0</span> lengths</span></div>
        <div class="zones" aria-hidden="true"><i style="flex:1200;background:#f2f4f1"></i><i style="flex:500;background:#3dbbcb"></i><i style="flex:800;background:#2a3a46"></i></div>
      </div>
    </div>
  </div></div>
</section>'''


def meet_stage(h2="Parents say yes. Coaches pick events. Hosts get their file."):
    return f'''
<section class="stage" data-len="420vh" data-steps="3" aria-label="How a meet moves through aquify">
  <div class="pin"><div class="split">
    <div class="txt">
      <h2 class="d2">{h2}</h2>
      <div class="steps">
        <div class="step">Parents commit from their phones<small>Each swimmer’s family answers yes or no. Yes opens the event list for that swimmer.</small></div>
        <div class="step">Coaches review every entry<small>Approve, change or reject with a reason before commitments lock.</small></div>
        <div class="step">Download the entry file<small>The meet’s entries, ready in the format the host asked for.</small></div>
      </div>
    </div>
    <div class="layers">
      {win("portal-meets", "Parent portal showing a meet that needs a response")}
      {win("meet-detail", "Meet page with commitment deadline and progress")}
      <div class="win" style="background:var(--deep)"><img src="assets/img/meet-detail.jpg" alt="Meet page, entries finalized" loading="lazy" decoding="async">
        <div class="fileflow" data-r="0.7,0.88"><i>SD3</i><div><b>Winter Invitational.sd3</b><br><span>Ready to send to the host</span></div></div></div>
    </div>
  </div></div>
</section>'''


def faq_group(title, items):
    d = "".join(f"<details><summary>{q}</summary><p>{a}</p></details>" for q, a in items)
    return f'<h2 class="d3">{title}</h2>{d}'


DARK = "#0a1117"
PAGES = {}

# =============== HOME ===============
PAGES["index.html"] = ("aquify: swim club software for coaches and admins", "Practice plans, attendance, meets, billing and parent messages in one app built for swim clubs.", f'''
<section class="hero">
  <canvas id="water" aria-hidden="true"></canvas><div class="fallback"></div>
  <div class="hero-in">
    <h1 class="d1">Coach the swimmers. We run the rest.</h1>
    <div class="hero-row">
      <p class="lede">Practice plans, attendance, meets, billing and parent messages in one app built for swim clubs.</p>
      <div style="display:flex;gap:12px;flex-wrap:wrap"><a class="btn" href="waitlist.html">Join the waitlist</a><a class="btn ghost" href="#start">See it work</a></div>
    </div>
  </div>
</section>
<span id="start"></span>
{rise("dashboard", "m-dash", "aquify dashboard showing commitments due, next practice and open invoices", [("Who hasn’t answered for the meet", "With the deadline counting down."), ("Which group swims next", "One tap to the workout or roll call."), ("Who still owes", "Open balances, by household.")])}
{say("Coaches write practice in shorthand. Parents commit to meets from their phones. Entries leave as the file the host asked for. Invoices go out in one run.", "320vh")}
{SHORTHAND}
{meet_stage()}
{zoom_stage("records", "Team records for each stroke, with the swimmer and the time they broke", "Import the results. The record book updates itself.")}
{fly([("m-portal", "Parent home screen on a phone"), ("m-billing", "Parent billing screen on a phone"), ("m-meets", "Parent meet list on a phone")], "Families see their household, and nothing else.", "Parents pay invoices, answer meet requests and sign up to volunteer. Swimmers get their own login to see their times and schedule.")}
{say("Whoever runs your club, there’s a screen built for the job.", "240vh")}
{index([("people.html#coaches", "For coaches", "Write workouts, run practice from your phone, take attendance and track training."), ("people.html#admins", "For club admins and registrars", "Approve sign-ups, build groups, run meets and keep the roster straight."), ("people.html#treasurers", "For treasurers", "Set dues, send invoices in one run, take card payments and see who owes."), ("people.html#parents", "For parents and swimmers", "Register, pay, respond to meets, volunteer and see times.")], links=True)}
<section class="sect tight"><div class="wrap"><h2 class="d2" style="max-width:14ch;margin-bottom:clamp(28px,5vh,60px)">Everything the season needs.</h2></div></section>
{index([(f, n, d) for f, n, d in FEATURES], links=True)}
{say("Your roster, your groups, your best times and your open invoices come with you.", "260vh")}
{index([("switch.html", "Switching to aquify", "What carries over from TeamUnify, SportsEngine Motion or a spreadsheet, and how we check it."), ("safety.html", "Safety and privacy", "Who sees what, how kids’ information is protected, and how families can get it deleted."), ("faq.html", "Questions", "Meet files, payments, phones, data and everything else coaches and admins ask.")], links=True)}
{cta()}
''')

# =============== COACHES ===============
PAGES["coaches.html"] = ("Coaches | aquify", "Write workouts in shorthand, run them on deck, take attendance and track training load.", f'''
{hero("Write the workout once.", "Plan it, assign it, run it on deck, and see who showed up.", maxch="10ch")}
{rise("training", "m-dash", "The training schedule listing every group’s practice", [("Every group’s practice, one list", "Filter to a single group."), ("Sets already attached", "Open the workout from the schedule."), ("Attendance a tap away", "Take it from the same row.")])}
{say("Write the set. Assign it to a group. Open it on deck. Mark who showed up.", "300vh")}
{SHORTHAND}
{steps_stage("Keep the sets that work.", [("Save any workout as a template", "Mark each set from easy recovery to all-out sprint, and see the mix at a glance."), ("Assign it to a practice", "Each practice gets its own copy, so changing Tuesday never changes the template."), ("Edit the sets in place", "Change a send-off or add a round and the yardage adds itself up again.")], [("library", "Workout library with saved templates and intensity bars"), ("training", "Practice schedule with workouts attached"), ("workout-sets", "Set editor showing yardage by section")])}
<section class="stage" data-len="340vh" data-steps="2" aria-label="Pool deck tools on a phone">
  <div class="pin"><div class="split">
    <div class="txt">
      <h2 class="d2">Made for the pool deck.</h2>
      <div class="steps">
        <div class="step">The workout, big enough to read from the water<small>Open any practice in deck view. Sets, send-offs and zones are set large for a phone held at arm’s length.</small></div>
        <div class="step">Roll call that survives bad wifi<small>Tap swimmers as they arrive. Marks save on the phone and sync when the connection returns.</small></div>
      </div>
    </div>
    <div class="layers phonebox">
      <div class="win">{phone("m-deck", "Workout in deck view on a phone", DARK, DARK, "rgba(255,255,255,.4)")}</div>
      <div class="win">{phone("m-attendance", "Roll call on a phone", DARK, DARK, "rgba(255,255,255,.4)")}</div>
    </div>
  </div></div>
</section>
{zoom_stage("analytics", "Training analytics: weekly yardage per group and attendance rate", "Weekly yardage, attendance and intensity, for every group.", "22%", "30%")}
{feature("wellness", "Wellness heatmap showing which swimmers checked in each day", "Know who needs a word.", "Swimmers, or their parents, answer a short check-in about sleep, soreness, energy, stress and mood. You see a grid of the last two weeks and who has been feeling off.", ["Takes about twenty seconds to fill in.", "Coaches and admins see the whole club. Families see only their own swimmer."], flip=True)}
{index([("Season plan", "Split the season into base, build and taper, and set a weekly yardage goal for each group."), ("Attendance", "See what share of practices each group shows up for. Excused absences don’t count against them."), ("Practice by group", "Parents and swimmers see the team-wide practices and their own group’s, nothing else."), ("Goals", "Set a time goal with a swimmer and watch the gap close as they race."), ("Relay builder", "Suggests the fastest four swimmers for a relay from their best times.")])}
{cta()}
''')

# =============== MEETS ===============
PAGES["meets.html"] = ("Meets | aquify", "Commitments, entries, host files, results import and team records for swim meets.", f'''
{hero("Meet day without the spreadsheet.", "Open a host’s file to build your meet. Send entries back in the format they expect.", chips='<div class="chips" aria-label="Meet file formats"><span class="chip hot">SD3</span><span class="chip">HY3</span><span class="chip">ZIP</span></div>')}
{rise("meet-detail", "m-meets", "Meet page with dates, commitment deadline and open items", [("Deadline and who hasn’t answered", "See every family still waiting."), ("Sessions and events from the host’s file", "Add any missing event by hand."), ("Entries and what’s still open", "Approve before the lock.")])}
{say("Open the host’s file. Collect commitments. Approve entries. Send the file back. Import the results.", "300vh")}
{meet_stage("From the family’s yes to the host’s file.")}
{steps_stage("Results come back in.", [("Matched to your swimmers", "aquify matches each swim to your swimmer by USA Swimming ID, then by name and birthday, and lists anything it couldn’t match."), ("Best times and top times update", "Every swimmer’s best times refresh, and the fastest swims by age are ready the same day.")], [("performance", "Best times for every swimmer"), ("top-times", "Top times by event for women, open age group")], "340vh")}
{zoom_stage("records", "Team records for each stroke", "Records are checked every time you import results.")}
{feature("p-volunteer", "Parent view of volunteer shifts for the meet", "Staff the meet from the same page.", "Post timer and runner shifts on the meet itself. Families sign up from their phones, and you see who’s covered and who’s still short.", ["Shifts have a limit, so a job is never overbooked.", "Hours count toward each family’s season requirement."], flip=True, phone_img="m-volunteer")}
{index([("Right events only", "Swimmers only see events their age and gender allow."), ("Coach changes", "Edit an entry before commitments close. aquify remembers who changed what."), ("Meet fees", "Charge a fee per event or a flat fee per swimmer, and it lands on the family’s invoice."), ("Protected records", "Mark a record as protected, and a faster swim waits for an admin’s okay before it replaces it."), ("Relay builder", "Suggests the fastest four swimmers for a relay from their best times."), ("Your own files", "Start a meet from SD3, HY3 or ZIP, and import results from any of them.")])}
{cta()}
''')

# =============== BILLING ===============
PAGES["billing.html"] = ("Billing | aquify", "Dues, sibling discounts, autopay and one-run invoicing for swim clubs.", f'''
{hero("Bill every household in one run.", "Set the dues once. Preview the run. Send the invoices.")}
{rise("invoices", "m-billing", "Invoice list with household, amount and due date", [("Every household’s invoice", "Amount, due date and status."), ("Paid, due or overdue", "Spot late balances at a glance."), ("One run, one preview", "Check it before it goes out.")])}
{say("Set the dues. Preview the run. Send the invoices. Get paid by card.", "280vh")}
{steps_stage("From dues to deposits.", [("Set dues, discounts and late fees", "A monthly fee for each group, or a one-time sign-up fee. Brothers and sisters get a discount, and discounts never pile on top of each other."), ("Preview first. Then bill.", "See exactly who will be charged before anything goes out. If something changes while you’re deciding, aquify asks you to look again. No family is ever billed twice for the same month."), ("Know who owes what", "See who’s behind and by how long. Send the numbers to QuickBooks.")], [("schedules", "Fee schedules, sibling discounts and late fees"), ("invoices", "Invoice list"), ("reports", "Receivables aging report")], "460vh")}
{feature("portal-billing", "Parent billing page with an outstanding balance and a Pay Now button", "Parents pay in two taps.", "Families see what they owe, what’s coming and what they’ve paid, then pay by card. The invoice marks itself paid.", ["Turn on autopay and they see the date and amount before every charge.", "A statement for each household shows every charge and payment.", "Card details go straight to Stripe. aquify never stores them."], flip=True)}
{index([("Autopay", "Families choose it, and always see the date and amount of the next charge."), ("Late fees", "One per invoice, added for you after the grace period you choose."), ("Credit on account", "If a family overpays, the extra is taken off their next invoice."), ("Refunds", "Refund a payment, or turn it into credit for the next invoice."), ("Joining mid-season", "Charge for the days left, half the fee, or nothing. You choose."), ("Meet and event fees", "Meet entry fees land on the family’s invoice, and event tickets get an invoice of their own."), ("Reports", "Money owed by age, revenue, deposits and a QuickBooks file.")])}
{zoom_stage("reports", "Receivables aging report", "Open balances, sorted by how late they are.", "20%", "18%")}
{cta()}
''')

# =============== REGISTRATION ===============
PAGES["registration.html"] = ("Registration | aquify", "Sign-ups, waivers, documents, groups and rosters for swim clubs.", f'''
{hero("Sign-ups without the paperwork pile.", "Families ask to join, you approve, and the roster builds itself.", maxch="12ch")}
{rise("registrations", "m-portal", "Registration list with swimmer, program, season and status", [("Every registration, one list", "By swimmer, group and season."), ("Waivers tracked for you", "See who has signed and who hasn’t."), ("Approve in one click", "Or withdraw a sign-up.")])}
{say("Families ask to join. You approve. They register their swimmers, sign the waiver and upload the forms.", "300vh")}
{steps_stage("From a stranger to a rostered swimmer.", [("Families ask to join", "A parent or coach fills in a short sign-up form. No setup call, no spreadsheet."), ("You approve", "New requests wait in one queue, with a count on the menu so you can’t miss them."), ("They register their swimmers", "Name, birthday, group and season, from their phone."), ("Waivers and forms", "Parents sign the waiver and upload documents. You see what’s missing for every swimmer."), ("The roster builds itself", "Activate a registration and the swimmer joins their group’s roster.")], [("approvals", "Pending approvals queue"), ("approvals", "Approvals queue"), ("p-regs", "Parent registration center"), ("p-docs", "Parent documents with a waiver waiting to be signed"), ("registrations", "Registration list")], numbered=True)}
{feature("households", "Household list showing contacts and swimmers", "Families are households, not rows.", "Each household has its parents, contacts and swimmers in one place. Add a second parent or an emergency contact, and everyone sees the same details.", ["A parent account is the one required login. Swimmers can have their own, but never need one.", "Swimmers carry their birthday, group and USA Swimming ID, so meet entries fill in correctly."], flip=True)}
{index([("Groups by age and level", "Set age ranges and a coach for each group, and add rules like age range or gender."), ("Seasons", "Open a season, set the dates and keep every registration tied to it."), ("Waiver tracking", "See which swimmers have a signed waiver at a glance."), ("Document types", "Ask for what your club needs. Each upload is checked, then marked verified or sent back."), ("Approvals queue", "Parents and coaches who ask to join wait for your yes."), ("Request to join", "Anyone can ask to join from your sign-up page, and nothing happens until you approve.")])}
{cta()}
''')

# =============== COMMUNICATION ===============
PAGES["communication.html"] = ("Communication | aquify", "Announcements, email newsletters and messages for swim clubs.", f'''
{hero("Say it once. Everyone hears it.", "Announcements, email and messages, with the right families on the other end.", maxch="10ch")}
{rise("comms", "m-comms", "Announcements list with published and draft posts", [("Post to the whole club or one group", "Parents see it in their portal."), ("Draft first, publish when ready", "Nothing goes out by accident."), ("Mark it public", "Show it on your club’s own page.")])}
{say("Announce it in the app. Email it to the club. Answer questions in a message. Families choose how they hear.", "300vh")}
{steps_stage("Three ways to reach families.", [("Announcements", "Post to everyone or to one group. Families see them on their home screen, and you choose which ones show on your public page."), ("Email newsletters", "Write once, pick the audience, send. aquify works out who gets it from your roster, so there’s no list to paste in and nobody to forget."), ("Messages", "Parents write to the club from their phone, and coaches answer in the same thread, with attachments.")], [("comms", "Announcements"), ("campaigns", "Email campaigns list"), ("p-comms", "Parent communications page")])}
{feature("p-comms", "Parent communications preferences", "Families pick what reaches them.", "Parents choose, for each kind of update, whether it comes by email or as a notification. Meet news can arrive instantly while billing notes stay in the portal.", ["Preferences belong to each person, so two parents can choose differently.", "Everything you post is still in the portal if someone turns the email off."], flip=True, phone_img="m-comms")}
{feature("portal-cal", "Family calendar with meets, practices and deadlines", "The family calendar follows them.", "Meets, practices, deadlines and club events appear in one calendar. A private link adds it to Apple, Google or Outlook, and it keeps itself current.", ["The link is private to the family and you can turn it off any time.", "Practices show only for the groups a family’s swimmers are in."], par=0.05)}
{index([("Send to a group", "Aim a message at one group, like Senior One, without anyone filtering a spreadsheet."), ("Delivery you can check", "See how many emails were sent, failed or skipped for each campaign."), ("Reminders for shifts", "Volunteers get an email the day before their shift."), ("Your sender name", "Emails come from your club’s name and address.")])}
{cta()}
''')

# =============== VOLUNTEERS & EVENTS ===============
PAGES["volunteers.html"] = ("Volunteers and events | aquify", "Volunteer shifts, service hours, club events, RSVPs and event fees for swim clubs.", f'''
{hero("Fill every shift. Count every hour.", "Volunteer jobs, service hours and club events, without the sign-up sheet.", maxch="11ch")}
{rise("volunteer", "m-volunteer", "Volunteer page with upcoming shifts and household hours", [("Every shift in order", "With how many spots are left."), ("Hours by household", "Against what each family owes."), ("Post a job in a minute", "Timers, runners, setup, concessions.")])}
{say("Post the job. Families pick a shift. You check them in. Their hours add up on their own.", "300vh")}
{steps_stage("From a sheet on the wall to a list that counts itself.", [("Post the shifts", "Set the role, time, place and how many people you need. Link it to a meet or a club event."), ("Families sign up", "From their phone, for their own household. Nobody can book the last spot twice, so you never end up over capacity."), ("Check in on the day", "Mark who showed up. Those hours go on the family’s record."), ("Hours add up", "See each household against the season’s requirement. Set a different requirement for one family if you need to.")], [("volunteer", "Admin volunteer page"), ("p-volunteer", "Parent shifts list"), ("volunteer", "Admin volunteer page"), ("p-volunteer", "Parent shifts list")], numbered=True)}
{feature("events", "Club events list with date, category and who is going", "Parties, banquets and clean-up days.", "Put any club event on the calendar and let families answer going, maybe or not going. Count guests and set a limit.", ["Add a fee, and every family that says yes gets an invoice for it. If they change their answer before paying, the invoice cancels itself.", "Events can be for the whole club or for chosen groups.", "Pick which events show on your public page."], flip=True)}
{feature("p-events", "Parent view of club events with RSVP buttons", "One tap to answer.", "Families see what’s coming and answer from their phone. Everyone who says yes shows in your list with their guest count.", ["Mark it all-day for dryland weekends or travel days.", "Events land in the family calendar automatically."], par=0.05, phone_img="m-events")}
{index([("Buy-outs", "Let a family pay instead of working a shift. aquify makes the invoice for you."), ("Cancel cutoff", "Set how close to a shift a family can still back out. Staff can always cancel."), ("Day-before reminders", "Each volunteer gets an email 24 hours before their shift."), ("Credits", "Give hours for a job outside the sign-up, like setup the night before."), ("Swimmers see theirs", "Older swimmers can see their family’s shifts without changing anything.")])}
{cta()}
''')

# =============== PERFORMANCE ===============
PAGES["performance.html"] = ("Performance | aquify", "Best times, time standards, team records, goals and progress for swim clubs.", f'''
{hero("Every swim, remembered.", "Best times, cuts, records and goals, built from the meets you already swim.", maxch="9ch")}
{rise("performance", "m-dash", "Best times for every swimmer by event", [("Best time for every event", "For every swimmer, updated after every meet."), ("Where it came from", "Imported result or entered by hand."), ("Search any swimmer", "Filter by distance or stroke.")])}
{say("Import the results. Best times update. Records are checked. Swimmers see how close they are to the next cut.", "300vh")}
{steps_stage("What happens when results come in.", [("Best times refresh", "Each swimmer’s best in each event updates the moment the file is imported."), ("Records are checked", "aquify compares every swim with the club record for that age group, stroke and course, and tells you which ones were broken."), ("Top times by age", "See the fastest swims in the club for any event and age group, any time of the year."), ("Swimmers see the next cut", "Pick a time standard and each swimmer sees whether they have made it, and how far they have to go.")], [("performance", "Best times"), ("records", "Team records"), ("top-times", "Top times"), ("swim-records", "Swimmer view of records")], numbered=True)}
{zoom_stage("records", "Team records for each stroke and age group", "Records by age group, with the whole history behind each one.")}
{feature("portal-records", "Parent view of team records and top times", "Everyone sees how fast the club is.", "Parents and swimmers get the same record board and top-times list. Age groups use the swimmer’s age on the day of the swim.", ["Protect a record, and a faster swim waits for an admin to approve.", "Add historical records from before you used aquify."], flip=True, par=0.05)}
{index([("Time standards", "Add cuts for championships or club goals, or import them from a file."), ("Goals", "Coaches, parents and swimmers can set a time goal and track it."), ("Progress report", "See how much faster a group got between two dates."), ("Relay builder", "Suggests the fastest four from best times, for medley and free relays."), ("Course aware", "Short course yards, short course meters and long course stay separate.")])}
{cta()}
''')

# =============== FAMILIES ===============
PAGES["families.html"] = ("Families | aquify", "Parent and swimmer portals: pay, respond to meets, volunteer and follow progress.", f'''
{hero("Parents get one place. Swimmers get theirs.", "Everything a family needs for the season, on the phone they already carry.", maxch="12ch")}
{fly([("m-portal", "Parent home on a phone"), ("m-billing", "Pay an invoice on a phone"), ("m-meets", "Answer a meet request on a phone")], "Pay, respond and sign up from the couch.", "Invoices, meet requests, volunteer shifts and documents, all on one home screen.")}
{say("Parents pay and respond from their phones. Swimmers see their own times. Nobody sees another family’s.", "300vh")}
{steps_stage("Built around the family.", [("The family calendar, in their calendar", "Meets, practices, deadlines and club events in one place. Add the private link to Apple, Google or Outlook Calendar and it keeps itself up to date."), ("Swimmers get their own login", "Their times, records, schedule and meet signups. They can edit a short bio and nothing else."), ("Parents stay in charge", "Sign-up, waivers, documents and payments stay with the parent.")], [("portal-cal", "Family calendar with meets, practices and deadlines"), ("swim-records", "Swimmer view of team records"), ("portal-billing", "Parent billing overview")], "440vh")}
{feature("p-regs", "Parent registration center", "Register once. Track everything.", "Parents see each swimmer’s registration, season and group, and exactly which waivers or documents are still missing.", ["Sign the waiver in the portal. The date it was accepted is recorded.", "Upload a form from the phone camera."], flip=True, par=0.05)}
{feature("p-wellness", "Wellness check-in for a swimmer", "A check-in that takes twenty seconds.", "Swimmers answer five quick questions before practice. Coaches spot who is run down before it turns into an injury.", ["Parents can fill it in for younger swimmers.", "Only coaches and the family see the answers."])}
{index([("Registration and waivers", "Parents sign up their swimmers, sign waivers and upload forms from their phone."), ("Messages and announcements", "Post to the whole club or one group. Families pick how they hear about each kind of update."), ("Volunteer shifts", "Families sign up for timers and runners and see their hours against the season requirement."), ("Club events", "RSVP, add guests and pay the event fee in the same place."), ("Your club’s public page", "A simple page with your about, contact details, news, upcoming meets and events."), ("Install it", "Add aquify to a phone’s home screen. No app store.")])}
<section class="sect">
  <div class="wrap row">
    <div class="txt"><h2 class="d2">Careful with kids’ information.</h2><p class="copy"><a href="safety.html" style="color:var(--chalk);text-decoration:underline;text-underline-offset:4px">Read how aquify protects it.</a></p></div>
    <div class="bigfacts">
      <div><b>Parents say yes first.</b><span>A swimmer under 13 can’t sign in until a parent okays it. A parent can take that back any time.</span></div>
      <div><b>Your family’s information is yours.</b><span>Parents can ask for a copy of everything the club has on their family, or ask for it to be deleted.</span></div>
      <div><b>Every change is written down.</b><span>When someone at the club changes something, aquify records who did it and when.</span></div>
    </div>
  </div>
</section>
{cta()}
''')

# =============== TEAM WEBSITE ===============
PAGES["website.html"] = ("Team website and app | aquify", "A public club page and an installable phone app, included with aquify.", f'''
{hero("Your club’s front door.", "A public page for new families, and an app on every phone. Both come with aquify.", maxch="9ch")}
{rise("club-site", "m-club", "A club’s public page with its name, about text and sign-in buttons", [("Your name and your story", "Tell new families who you are."), ("Join and sign in", "Straight into aquify."), ("News and meets", "Only what you choose to show.")])}
{say("Turn it on. Pick what shows. New families find you, and your members sign in from the same page.", "280vh")}
{steps_stage("You choose what’s public.", [("Pick your address", "Choose a short web address for your club, like harbor-aquatic."), ("Write your about and contact", "A few lines about the club, plus an email and phone number."), ("Tick what shows", "Mark announcements and events as public one by one. Everything else stays inside."), ("Turn it off any time", "Switch the page off and it disappears.")], [("settings", "Public site settings"), ("settings", "Public site settings"), ("comms", "Announcements with a public option"), ("club-site", "The public club page")], numbered=True)}
{feature("club-site", "Public club page", "What visitors see.", "Your club’s name, your story, how to reach you, any announcements and events you’ve marked public, and the next meets. Buttons take families straight to join or sign in.", ["No swimmer names or times ever appear on it.", "Coach names show only if you turn that on."], flip=True, par=0.05)}
<section class="stage fly" data-len="320vh">
  <div class="pin"><div class="phones">
    <div class="phone" data-r="0.04,0.4" style="--r:-7deg"><div class="screen" style="--sb:#0a1117;--sbb:#0a1117;--ind:rgba(255,255,255,.4)"><img src="assets/img/m-club.jpg" alt="Public club page on a phone" loading="lazy" decoding="async"></div></div>
    <div class="phone" data-r="0.18,0.54" style="--r:0deg"><div class="screen" style="--sb:#fcfbfa;--sbb:#f1e7d8"><img src="assets/img/m-portal.jpg" alt="Parent home on a phone" loading="lazy" decoding="async"></div></div>
    <div class="phone" data-r="0.32,0.68" style="--r:7deg"><div class="screen" style="--sb:#0a1117;--sbb:#0a1117;--ind:rgba(255,255,255,.4)"><img src="assets/img/m-deck.jpg" alt="Workout in deck view on a phone" loading="lazy" decoding="async"></div></div>
  </div>
    <div class="flycap" data-r="0.68,0.86"><h2 class="d3" style="max-width:15ch">One app. Three kinds of phone screen.</h2><p class="lede" style="max-width:34ch">Families, coaches and your public page all run from the browser, and install to the home screen.</p></div>
  </div>
</section>
{index([("Install it", "Open aquify on a phone and add it to the home screen. It opens like an app, with no app store."), ("Works on poor wifi", "Roll call marks are saved on the phone and sent when the signal returns."), ("One address per club", "Your page lives at a short link you can put on a banner or a flyer."), ("Signed-in pages stay private", "Only the public page is visible without a login.")])}
{cta()}
''')

# =============== A SEASON ===============
PAGES["season.html"] = ("A season with aquify | aquify", "See how a swim season runs in aquify, from registration to the record board.", f'''
{hero("A season, start to finish.", "Follow one swim season through aquify, from the first sign-up to the last record.", maxch="9ch")}
{steps_stage("Here’s how a season runs.", [("Open registration", "Families ask to join, you approve, and they register their swimmers."), ("Build your groups", "Set age ranges, assign coaches, and the roster fills as registrations go active."), ("Plan the practices", "Write workouts in shorthand and assign them to each group’s week."), ("Take attendance", "Roll call on the pool deck. Attendance builds the training picture."), ("Collect meet commitments", "Families answer yes or no and pick events, with coaches adjusting before the lock."), ("Send entries, import results", "The entry file goes to the host. After the meet, results come back in."), ("Bill the club", "Preview the run, send the invoices, and take card payments."), ("Watch the records fall", "Best times and records update from every meet.")], [("approvals", "Approvals"), ("registrations", "Registrations"), ("library", "Workout library"), ("attendance", "Roll call"), ("portal-meets", "Parent meet commitment"), ("meet-detail", "Meet page"), ("invoices", "Invoices"), ("records", "Team records")], "1100vh", numbered=True)}
{say("Then next season starts, and everything you set up is already there.", "240vh")}
{index([("registration.html", "Registration", "Sign-ups, waivers, groups and rosters."), ("coaches.html", "Coaches", "Workouts, roll call and training."), ("meets.html", "Meets", "Entries, results and records."), ("billing.html", "Billing", "Dues, discounts and invoices.")], links=True)}
{cta()}
''')

# =============== PEOPLE (role pages as anchored sections) ===============
PAGES["people.html"] = ("Who it’s for | aquify", "How coaches, club admins, treasurers and families use aquify.", f'''
{hero("Built for whoever runs the club.", "Coaches, registrars, treasurers and parents each get the screens they need, and nothing else.", maxch="12ch")}
<span id="coaches"></span>
{rise("training", "m-dash", "Coach view of the practice schedule", [("Your practices", "Every group, one list."), ("Your workouts", "Written, assigned, ready."), ("Your attendance", "Tap to mark.")])}
{index([("Write workouts in shorthand", "Type the set, and aquify adds up the yardage."), ("Run practice from your phone", "Deck view and offline roll call."), ("Approve meet entries", "Coaches review before the lock."), ("See who’s tired", "Wellness check-ins flag swimmers who need a word.")])}
<section class="sect tight"><div class="wrap"><a class="btn ghost" href="coaches.html">See everything for coaches</a></div></section>
<span id="admins"></span>
{say("Club admins and registrars: approve sign-ups, build groups, run meets, keep the roster straight.", "260vh")}
{index([("Approve sign-ups", "One queue, with a count on the menu."), ("Groups and rosters", "Age ranges, coaches and rules."), ("Run the meet", "Commitments, entries, files."), ("Post news", "Announcements and email to the right group."), ("Settings", "Team code, sender name, public page.")])}
<section class="sect tight"><div class="wrap"><a class="btn ghost" href="registration.html">See registration</a> <a class="btn ghost" href="meets.html">See meets</a></div></section>
<span id="treasurers"></span>
{rise("invoices", "m-billing", "Invoice list for a treasurer", [("Everyone who owes", "By how late."), ("One run", "To bill the club."), ("One file", "For QuickBooks.")])}
{index([("Set dues once", "By group, with sibling discounts and late fees."), ("Preview, then send", "One run bills every household."), ("Take card payments", "Parents pay, invoices update."), ("Refund or credit", "Fix a mistake without a spreadsheet.")])}
<section class="sect tight"><div class="wrap"><a class="btn ghost" href="billing.html">See billing</a></div></section>
<span id="parents"></span>
{say("Parents and swimmers: register, pay, answer meet requests, volunteer and follow times, all from a phone.", "260vh")}
{index([("Register and sign waivers", "From the phone."), ("Pay invoices", "Or turn on autopay."), ("Answer for the meet", "Yes or no, then pick events."), ("Volunteer", "Pick a shift that fits."), ("Follow times", "Best times, records and goals.")])}
<section class="sect tight"><div class="wrap"><a class="btn ghost" href="families.html">See the family portal</a></div></section>
{cta()}
''')

# =============== SWITCH ===============
PAGES["switch.html"] = ("Switching to aquify | aquify", "What carries over from TeamUnify, SportsEngine Motion or spreadsheets, and how we check it.", f'''
{hero("Bring your club with you.", "Your roster, groups, best times and open invoices move over from the exports you already have.", maxch="9ch")}
{rise("households", "m-dash", "Household list after import", [("Your households", "With their parents and contacts."), ("Your swimmers", "In the right groups."), ("Your open balances", "Matching what you had.")])}
{say("You don’t start from zero. You bring the roster, the groups, the best times and what families still owe.", "300vh")}
{steps_stage("How we move your club over.", [("Send us your exports", "Download spreadsheets of your households, swimmers, groups, best times, open invoices and payments from your current system."), ("We import them", "We load everything into aquify as a practice run first, so nothing real is at stake."), ("We check the numbers", "Counts of households and swimmers match your export, and the money families owe adds up to the same total."), ("You look it over", "Walk through your own club in aquify, fix what’s off, and decide when you’re ready."), ("Families sign in", "Parents get access to their own household. Swimmers can follow once a parent says yes.")], [("households", "Households"), ("members", "Swimmers"), ("invoices", "Open invoices"), ("dashboard", "Dashboard"), ("p-household", "Parent household")], numbered=True)}
{index([("Households and contacts", "Parents, emergency contacts and emails."), ("Swimmers and groups", "Names, birthdays, USA Swimming IDs and who swims where."), ("Best times", "From an export, or rebuilt from your results files."), ("Open invoices", "With due dates and what’s still owed."), ("Payment history", "So balances line up."), ("Meet files", "Bring in SD3, HY3 and ZIP results to rebuild history.")])}
{feature("settings", "Settings page with team code and sender identity", "Set up in an afternoon, not a season.", "Add your team code, your sender name and your public page. Then invite coaches and approve the first families.", ["We’re still finalizing how onboarding works. Tell us what you use today when you join the waitlist."], flip=True)}
{cta("Tell us what you use today.", "Join the waitlist and say which system you’re coming from. We’ll plan your move around it.")}
''')

# =============== SAFETY ===============
PAGES["safety.html"] = ("Safety and privacy | aquify", "Who can see what, how kids’ information is protected, and how families can get it deleted.", f'''
{hero("Careful with kids’ information.", "Swimmers are children. aquify is built around that.", maxch="10ch")}
{say("Each person sees what their job needs. Nothing more.", "240vh")}
{index([("Admins", "Run the club. Sensitive actions, like approving a deletion, ask for a second sign-in step."), ("Coaches", "See their groups, practices, meets and entries. They don’t see club finances."), ("Parents", "See their own household: swimmers, invoices, documents and meet requests. Never another family’s."), ("Swimmers", "See their own times and schedule. They can edit a short bio and nothing else.")])}
{steps_stage("What protects the kids.", [("Parents say yes first", "A swimmer under 13 can’t sign in until a parent records consent. A parent can take it back, and the account is unlinked."), ("Public pages show no swimmers", "Your club’s public page never lists swimmer names or times."), ("Every change is written down", "aquify records who changed what and when, and keeps it for years."), ("Families can ask for their data", "A parent can request a copy of their household’s information, or ask for it to be deleted. An admin reviews every deletion.")], [("p-household", "Household page"), ("club-site", "Public club page"), ("audit", "Audit log"), ("p-household", "Household page")], numbered=True)}
{feature("audit", "Audit log of club activity", "A record of every change.", "When someone at the club creates, edits or approves something, the log keeps the who, what and when. You can search it and read it any time.", ["Kept for about seven years.", "Filter by what happened or by type."], flip=True)}
{index([("Card details stay with Stripe", "Parents pay on Stripe’s page. aquify never sees or stores a card number."), ("No ads, no selling data", "Information about your club and its swimmers is never sold or used for advertising."), ("Backups", "Encrypted backups are kept for about 30 days."), ("Hosted in the United States", "Your club’s information is stored on servers in the US."), ("Sign-in and roles", "Everyone signs in. What they can do depends on their role, and anything not allowed is denied by default.")])}
{cta("Questions about privacy?", "Join the waitlist and ask. We’ll answer before your club decides.")}
''')

# =============== FAQ ===============
PAGES["faq.html"] = ("Questions | aquify", "Answers to what coaches and club admins ask about aquify.", f'''
{hero("Questions coaches and admins ask.", "Straight answers. If something isn’t decided yet, we say so.", maxch="12ch")}
<section class="sect tight"><div class="wrap faq">
{faq_group("The basics", [
("Who is aquify for?", "Swim clubs. Coaches use it for workouts and attendance, admins for sign-ups and meets, treasurers for billing, and parents and swimmers for their own pages."),
("What will it cost, and when can we start?", "We haven’t set pricing or a launch date. Join the waitlist and we’ll email you before either is final."),
("Does it replace TeamUnify or SportsEngine Motion?", "That’s what it’s built to do. It covers registration, meets, billing, messaging and training in one place. Some clubs will want to move everything at once, others in stages."),
("Do we need to be technical?", "No. If you can use email and a spreadsheet, you can run aquify. Each role sees only the screens it needs.")])}
{faq_group("Meets", [
("Which meet files does it work with?", "You can build a meet from an SD3, HY3 or ZIP file, import results from any of them, and send your entries back as an SD3 file."),
("Can parents choose events?", "Yes. A parent answers yes or no, and a yes opens the events that swimmer is allowed to swim. Coaches can change entries before they lock."),
("What if a swim doesn’t match one of our swimmers?", "aquify matches by USA Swimming ID first, then name and birthday. Anything it can’t match is listed so you can fix it.")])}
{faq_group("Money", [
("How do parents pay?", "By card. They pay on Stripe’s page and the invoice marks itself paid. Families can also turn on autopay."),
("Does aquify store card numbers?", "No. Card details go straight to Stripe."),
("Can we do sibling discounts, late fees and registration fees?", "Yes to all three. Discounts apply the best offer for each swimmer and never stack. Late fees are added once per invoice."),
("Can I get the numbers into QuickBooks?", "Yes. aquify makes a QuickBooks file, and the money reports export as spreadsheets.")])}
{faq_group("Families and phones", [
("Do swimmers need their own accounts?", "No. A parent account is all a family needs. Swimmers can have their own login if the club turns it on, and if they’re under 13 a parent has to say yes first."),
("Can parents see other families?", "Never. A parent sees only their own household."),
("Is there an app in the app store?", "No. aquify installs from the browser to a phone’s home screen and opens like an app."),
("Does roll call work without wifi?", "Yes. Marks save on the phone and sync when the connection comes back.")])}
{faq_group("Your data", [
("Where is our information stored?", "On servers in the United States."),
("Can we get our data out?", "Yes. The money and roster reports export as spreadsheets, and a family can request a copy of their household’s information."),
("What happens if a family wants their data deleted?", "The parent asks, an admin reviews it, and aquify deletes or de-identifies it. Invoices and payments stay as accounting rules require."),
("Who can see what?", "Admins see everything. Coaches see their groups, practices and meets. Parents see their own household. Swimmers see their own times and schedule.")])}
{faq_group("Switching", [
("How do we move from our current system?", "Download your households, swimmers, groups, best times, open invoices and payment history from your current system, and we’ll import them and check the totals with you. The <a href='switch.html' style='text-decoration:underline;text-underline-offset:4px'>switching page</a> has the details."),
("What isn’t ready yet?", "Pricing and a launch date aren’t set, and we’re still finalizing how moving over works.")])}
</div></section>
{cta("Still have a question?", "Join the waitlist and ask. We read every message.")}
''')

# =============== WAITLIST ===============
PAGES["waitlist.html"] = ("Join the waitlist | aquify", "Be first when your swim club can switch to aquify.", f'''
<section class="page-hero" style="min-height:100svh">{WATER}<div class="in" style="grid-template-columns:minmax(0,6fr) minmax(0,5fr);gap:clamp(32px,6vw,120px);align-items:end">
  <div style="display:grid;gap:32px"><h1 class="d1" style="font-size:clamp(48px,min(8.4vw,15svh),150px);max-width:8ch">Join the waitlist.</h1>
  <p class="lede">We’re opening aquify to clubs in order. There’s no price or launch date yet, so we’ll email you when it’s your club’s turn.</p></div>
  <form class="wl" data-endpoint="" novalidate>
    <div class="field"><label for="name">Your name</label><input id="name" name="name" autocomplete="name" required></div>
    <div class="field"><label for="email">Email</label><input id="email" name="email" type="email" autocomplete="email" required></div>
    <div class="field"><label for="club">Club name</label><input id="club" name="club" autocomplete="organization" required></div>
    <div class="field"><label for="role">Your role</label><select id="role" name="role"><option>Head coach</option><option>Assistant coach</option><option>Club administrator</option><option>Treasurer or registrar</option><option>Parent volunteer</option><option>Other</option></select></div>
    <div class="field"><label for="system">What do you use today?</label><input id="system" name="system" placeholder="TeamUnify, SportsEngine, spreadsheets…"></div>
    <button class="btn" type="submit">Join the waitlist</button>
    <p class="msg" role="status" aria-live="polite"></p>
  </form>
</div></section>
<style>@media(max-width:900px){{.page-hero .in{{grid-template-columns:1fr!important}}}}</style>
''')

for f, (t, d, b) in PAGES.items():
    (OUT / f).write_text(shell(f, t, d, b), encoding="utf-8")
print("wrote", len(PAGES), "pages")
