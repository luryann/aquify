import type { Metadata } from 'next'
import Link from 'next/link'
import { LegalDoc, type LegalSection } from '@/components/legal/LegalDoc'
import { LEGAL_CONTACT_EMAIL, TERMS_PATH } from '@/lib/legal'

export const metadata: Metadata = {
  title: 'Privacy policy',
  description: 'What aquify collects about swim club families and swimmers, why, who sees it, and the choices parents have.',
}

const email = <a href={`mailto:${LEGAL_CONTACT_EMAIL}`}>{LEGAL_CONTACT_EMAIL}</a>

const sections: LegalSection[] = [
  {
    id: 'who-we-are',
    title: 'Who we are and what this covers',
    body: (
      <>
        <p>
          aquify is software that swim clubs use to run their programs: rosters, meets, practices, billing, documents and
          messages. This policy explains how aquify (“we”, “us”) handles personal information in that software, including
          the parent, swimmer and coach portals and the public pages we host for clubs.
        </p>
        <p>
          Each club decides which families and swimmers it enrolls and what it asks them to provide. We process that
          information to run the service for the club and for the families in it. The club is responsible for its own
          decisions about its roster and records; we are responsible for operating the platform securely and for the
          practices described here.
        </p>
      </>
    ),
  },
  {
    id: 'what-we-collect',
    title: 'What we collect',
    body: (
      <>
        <p>We collect information in four ways: from parents and guardians, from the club, from swimmers who have their own account, and automatically when you use the service.</p>
        <table className="lg-table">
          <thead>
            <tr><th scope="col">Information</th><th scope="col">Examples</th><th scope="col">Comes from</th></tr>
          </thead>
          <tbody>
            <tr>
              <td>Account details</td>
              <td>Name, email address, phone number, role (parent, swimmer, coach, admin), sign-in and security settings</td>
              <td>You or your club</td>
            </tr>
            <tr>
              <td>Swimmer profile</td>
              <td>Name, preferred name, date of birth, gender, USA Swimming ID, registration status, group and team assignments, a short bio the swimmer or parent writes</td>
              <td>Parent or club</td>
            </tr>
            <tr>
              <td>Health and safety</td>
              <td>Emergency contact, allergies, medical notes and flags, signed waivers and uploaded documents</td>
              <td>Parent or club</td>
            </tr>
            <tr>
              <td>Swimming activity</td>
              <td>Meet commitments and entries, times and results, attendance, goals, training plans, volunteer hours, optional wellness check-ins</td>
              <td>Club, parent, swimmer, meet files the club imports</td>
            </tr>
            <tr>
              <td>Billing</td>
              <td>Invoices, payments, credits, autopay settings. Card numbers go directly to our payment processor and are not stored by us</td>
              <td>Parent, club, payment processor</td>
            </tr>
            <tr>
              <td>Messages</td>
              <td>Announcements, emails and notification preferences</td>
              <td>Club, you</td>
            </tr>
            <tr>
              <td>Technical records</td>
              <td>IP address, browser type, sign-in times, and a log of administrative actions. We use cookies only to keep you signed in and protect against forged requests</td>
              <td>Collected automatically</td>
            </tr>
          </tbody>
        </table>
        <p>We do not use advertising or cross-site tracking technology, and we do not build advertising profiles.</p>
      </>
    ),
  },
  {
    id: 'children',
    title: 'Children’s privacy',
    body: (
      <>
        <p>
          Most information about swimmers is entered by their parents or guardians or by the club, and a parent account is
          required for registration, meet commitments, billing and documents. A swimmer does not need an account of their
          own to take part.
        </p>
        <p>
          A club may offer swimmers their own sign-in so they can see their schedule, times, goals and messages from coaches.
          <strong> A swimmer under 13 can use their own account only after a parent or guardian gives consent in the parent
          portal.</strong> Without recorded consent, the account cannot be linked and cannot sign in. We record who gave
          consent, when, and which version of this policy they saw.
        </p>
        <h3>What a child with an account can do and share</h3>
        <ul>
          <li>View their own schedule, meets, times, records and messages from the club.</li>
          <li>Set goals, submit optional wellness check-ins, and edit a bio of up to 400 characters.</li>
          <li>Nothing a child writes is published publicly. aquify does not offer public profiles, open chat, or contact with people outside the club.</li>
        </ul>
        <p>
          We ask a child for only what the features above need. We do not require a child to provide more information than
          is reasonably necessary to take part. We do not sell children’s information, use it for advertising, or share it
          with third parties other than the service providers listed below.
        </p>
        <h3>Your choices as a parent or guardian</h3>
        <ul>
          <li><strong>Review:</strong> see everything stored about your child in the parent portal, or ask for a copy of it.</li>
          <li><strong>Withdraw consent:</strong> turn off your child’s own account at any time on the swimmer’s page in the parent portal. This unlinks their sign-in and stops further collection through that account.</li>
          <li><strong>Delete:</strong> ask us or your club to delete your child’s information (see “Your rights and choices”).</li>
        </ul>
        <p>
          If we learn that we collected information from a child under 13 without the required parental consent, we will
          delete it. If you believe this has happened, email {email}.
        </p>
      </>
    ),
  },
  {
    id: 'how-we-use',
    title: 'How we use information',
    body: (
      <>
        <ul>
          <li>Run the club’s program: rosters, practices, meet entries, results, attendance and training.</li>
          <li>Process registrations, invoices and payments, and send receipts and reminders.</li>
          <li>Send messages the club or aquify needs you to see, honoring your notification preferences.</li>
          <li>Produce files that meets and USA Swimming require, such as entry files, and import results from them.</li>
          <li>Keep the service secure: detect misuse, enforce access rules, and keep an audit trail of administrative actions.</li>
          <li>Support you, fix problems, and meet legal obligations.</li>
        </ul>
        <p>We do not use personal information for advertising and we do not sell it.</p>
      </>
    ),
  },
  {
    id: 'sharing',
    title: 'Who we share it with',
    body: (
      <>
        <p>
          Within a club, access depends on role. Parents see their own household. Swimmers see their own information.
          Coaches and administrators see the information they need to run the program. Medical details are limited to staff
          and the swimmer’s own household.
        </p>
        <p>We use these service providers, who may handle information only to provide their service to us:</p>
        <table className="lg-table">
          <thead>
            <tr><th scope="col">Provider</th><th scope="col">Purpose</th></tr>
          </thead>
          <tbody>
            <tr><td>Stripe</td><td>Card payments, checkout and refunds</td></tr>
            <tr><td>Postmark</td><td>Sending email (receipts, reminders, announcements)</td></tr>
            <tr><td>Cloudflare</td><td>Network security and traffic delivery</td></tr>
            <tr><td>Our hosting and storage providers</td><td>Servers, database, file storage and backups, located in the United States</td></tr>
          </tbody>
        </table>
        <p>
          If your club connects aquify to <strong>USA Swimming</strong>, we send member identifiers and registration details
          to USA Swimming and receive membership status back. This happens only when the club enables the connection, and
          is limited to what USA Swimming’s program requires to register and verify members.
        </p>
        <p>
          We may also disclose information when required by law, to protect someone’s safety or our users’ rights, or
          as part of a merger or sale of our business, in which case the policy protections here continue to apply to
          the information transferred.
        </p>
      </>
    ),
  },
  {
    id: 'retention',
    title: 'How long we keep it',
    body: (
      <>
        <ul>
          <li><strong>Account and swimmer records:</strong> while the family’s account is active with the club.</li>
          <li><strong>After an approved deletion request:</strong> we delete or de-identify the household’s information, except what we must keep, as described next.</li>
          <li><strong>Financial records:</strong> invoices, payments and related ledger entries are kept as tax and accounting law requires.</li>
          <li><strong>Audit logs:</strong> records of administrative actions are kept for up to seven years for security and legal purposes.</li>
          <li><strong>Backups:</strong> encrypted backups roll off on a schedule of about 30 days, so deleted information leaves backups within that time.</li>
          <li><strong>Data exports:</strong> files we generate for you expire automatically after a short period.</li>
        </ul>
        <p>We keep information about a child only as long as reasonably necessary for the purposes in this policy.</p>
      </>
    ),
  },
  {
    id: 'security',
    title: 'How we protect it',
    body: (
      <>
        <ul>
          <li>Connections are encrypted in transit and traffic passes through a web application firewall.</li>
          <li>Access is deny-by-default and limited by role and household. Administrators use multi-factor authentication for sensitive actions.</li>
          <li>Sign-in session tokens are stored hashed. Credentials for connected services are stored encrypted.</li>
          <li>Administrative and privileged actions are written to an audit log.</li>
          <li>Backups are encrypted, and we maintain a written incident-response procedure.</li>
        </ul>
        <p>
          No system is perfectly secure. If a breach affects your information, we will notify you and the club, and
          regulators where required, within the time the law allows.
        </p>
      </>
    ),
  },
  {
    id: 'rights',
    title: 'Your rights and choices',
    body: (
      <>
        <p>You can ask us, or your club, to:</p>
        <ul>
          <li>tell you what information we hold about you or your child, and give you a copy;</li>
          <li>correct information that is wrong;</li>
          <li>delete information, subject to the retention exceptions above;</li>
          <li>stop further collection from your child by withdrawing consent.</li>
        </ul>
        <p>
          Email {email} or ask your club administrator. We may need to confirm you are the parent or guardian, or the
          person the information is about, before we act. We respond within 45 days. Parents can also manage their
          household’s details and notification preferences directly in the parent portal. You will not be treated worse
          for using these rights.
        </p>
        <h3>California residents</h3>
        <p>
          Under California law you may ask what personal information we collect, use and disclose, and ask us to correct or
          delete it. We do not sell personal information or share it for cross-context behavioral advertising, so there is
          nothing to opt out of. You may use an authorized agent to make a request on your behalf.
        </p>
        <h3>Outside the United States</h3>
        <p>
          aquify is built for swim clubs in the United States and stores information there. It is not directed to people in
          the European Economic Area or United Kingdom. If you are located there and believe your information is in aquify,
          contact us at {email} and we will address your request.
        </p>
      </>
    ),
  },
  {
    id: 'changes',
    title: 'Changes to this policy',
    body: (
      <>
        <p>
          We will post changes here with a new effective date and notify account holders by email or in the app before any
          material change takes effect. If a change affects how we handle children’s information, we will ask parents
          for consent again where the law requires it.
        </p>
        <p>
          Our <Link href={TERMS_PATH}>Terms of service</Link> also apply to your use of aquify.
        </p>
      </>
    ),
  },
]

const summary = {
  items: [
    { term: 'What we hold about swimmers', text: 'The details a parent or club enters, such as name, birth date, health notes and emergency contacts, plus meet results, attendance and billing.' },
    { term: 'Children under 13', text: 'They can have their own account only after a parent says yes in the parent portal. A parent can take that back at any time.' },
    { term: 'What we never do', text: 'Sell personal information, show ads, or publish a swimmer’s name or results on the open internet.' },
    { term: 'Who helps us run it', text: 'A small set of providers for payments, email, network security and hosting, plus USA Swimming when your club connects to it.' },
    { term: 'Your say', text: 'Ask to see, fix or delete your family’s information at any time.' },
  ],
}

export default function PrivacyPage() {
  return (
    <LegalDoc
      current="privacy"
      title="Privacy policy"
      summary={summary}
      sections={sections}
    />
  )
}
