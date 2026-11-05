import type { Metadata } from 'next'
import Link from 'next/link'
import { LegalDoc, type LegalSection } from '@/components/legal/LegalDoc'
import { LEGAL_CONTACT_EMAIL, PRIVACY_PATH } from '@/lib/legal'

export const metadata: Metadata = {
  title: 'Terms of service',
  description: 'The rules for using aquify as a parent, swimmer, coach or club administrator.',
}

const sections: LegalSection[] = [
  {
    id: 'agreement',
    title: 'Agreeing to these terms',
    body: (
      <>
        <p>
          These terms govern your use of aquify, the software your swim club uses to manage its program. By creating an
          account or using aquify you agree to them and to our <Link href={PRIVACY_PATH}>Privacy policy</Link>. If you do not
          agree, do not use the service.
        </p>
        <p>If you use aquify on behalf of a club, you confirm that you are authorized to accept these terms for it.</p>
      </>
    ),
  },
  {
    id: 'accounts',
    title: 'Accounts and who can have one',
    body: (
      <>
        <ul>
          <li><strong>Parents and guardians</strong> must be at least 18. A parent account is required to register a swimmer, respond to meets, pay invoices and submit documents.</li>
          <li><strong>Swimmers</strong> may have their own account only if the club offers it. A swimmer under 13 needs a parent’s recorded consent first, and a parent may withdraw it at any time.</li>
          <li><strong>Coaches and administrators</strong> are given access by their club and may use it only for that club’s program.</li>
        </ul>
        <p>
          Give accurate information and keep it current. Keep your sign-in details private and tell us promptly if you think
          someone else has used your account. You are responsible for activity under your account and for the swimmers in
          your household.
        </p>
      </>
    ),
  },
  {
    id: 'club-role',
    title: 'The club’s role',
    body: (
      <p>
        Your club runs its own program: it decides who is enrolled, sets fees, schedules, policies and waivers, and decides
        what information it asks families to provide. aquify provides the tools. Questions about fees, refunds, selection,
        safety rules or a club’s decisions are for the club. The club is responsible for using aquify lawfully and for
        having the right to enter the information it adds.
      </p>
    ),
  },
  {
    id: 'acceptable-use',
    title: 'Acceptable use',
    body: (
      <>
        <p>You agree not to:</p>
        <ul>
          <li>access information about households or swimmers you are not authorized to see;</li>
          <li>upload unlawful, harmful or infringing content, or malware;</li>
          <li>probe, overload or try to bypass security or access controls, or scrape the service;</li>
          <li>use member or swimmer information for marketing, resale, or anything beyond the club’s program;</li>
          <li>impersonate another person or misstate your role or relationship to a swimmer.</li>
        </ul>
        <p>We may suspend or limit accounts that put swimmers, other users or the service at risk.</p>
      </>
    ),
  },
  {
    id: 'payments',
    title: 'Payments',
    body: (
      <p>
        Fees are set and collected by your club. Card payments are processed by Stripe under its own terms, and aquify does
        not store full card numbers. When you set up autopay or pay an invoice you authorize the charge shown to you.
        Refunds, credits and late fees follow your club’s policy and the invoice terms.
      </p>
    ),
  },
  {
    id: 'content',
    title: 'Your information and content',
    body: (
      <>
        <p>
          You keep ownership of the information and files you add. You give aquify and your club permission to host,
          process and display them as needed to provide the service, as described in the Privacy policy. You promise you have
          the right to add it, including a parent’s authority to provide information about their child.
        </p>
        <p>
          aquify and its licensors own the software, design and documentation. You may use them only as aquify allows
          you to under these terms.
        </p>
      </>
    ),
  },
  {
    id: 'third-parties',
    title: 'Connected services',
    body: (
      <p>
        aquify can connect to outside services such as payment processing, email delivery, and USA Swimming’s member
        systems. Those services have their own terms, availability and data rules, and a club must be authorized by them
        to connect. Membership and time information from USA Swimming is provided by USA Swimming, and aquify cannot
        guarantee that it is complete or current.
      </p>
    ),
  },
  {
    id: 'safety',
    title: 'Health and safety information',
    body: (
      <p>
        Medical notes, allergies and emergency contacts in aquify help coaches prepare, but aquify is not an emergency
        service and does not replace a club’s safety procedures. In an emergency, call 911 first. Check that the information
        for your swimmer is accurate before each season.
      </p>
    ),
  },
  {
    id: 'availability',
    title: 'Availability and changes',
    body: (
      <p>
        We work to keep aquify available but do not promise uninterrupted service, and we may change, add or remove
        features. We will give reasonable notice of changes that significantly affect how you use it.
      </p>
    ),
  },
  {
    id: 'ending',
    title: 'Ending your use',
    body: (
      <>
        <p>
          You can stop using aquify at any time, and you can ask for your household’s information to be deleted as
          described in the Privacy policy. We or your club may suspend or end access for violations of these terms, for
          security reasons, or when the club ends its use of aquify. Sections that by their nature should continue after
          that, such as ownership, limits of liability and disputes, will continue.
        </p>
      </>
    ),
  },
  {
    id: 'liability',
    title: 'Warranties and liability',
    body: (
      <>
        <p>
          aquify is provided “as is” and “as available.” To the fullest extent the law allows, we disclaim all warranties,
          express or implied, including fitness for a particular purpose and non-infringement.
        </p>
        <p>
          To the fullest extent the law allows, aquify will not be liable for indirect, incidental, special or
          consequential damages, or lost profits or data, and our total liability for any claim relating to the service
          will not exceed the amount you paid aquify, if any, in the 12 months before the claim. Nothing in these terms
          limits liability that cannot legally be limited, including for fraud or for personal injury caused by negligence.
        </p>
      </>
    ),
  },
  {
    id: 'disputes',
    title: 'Disputes and governing law',
    body: (
      <p>
        These terms are governed by the laws of the United States and of the state where aquify’s operator is organized,
        without regard to conflict-of-law rules. Before filing a claim, please contact us at{' '}
        <a href={`mailto:${LEGAL_CONTACT_EMAIL}`}>{LEGAL_CONTACT_EMAIL}</a> so we can try to resolve it informally. Any
        claim must be brought in the state or federal courts located in that state, and you and aquify consent to
        their jurisdiction.
      </p>
    ),
  },
  {
    id: 'changes',
    title: 'Changes to these terms',
    body: (
      <p>
        We may update these terms. We will post the new version with its effective date and tell account holders before
        material changes take effect. Continuing to use aquify after that means you accept the updated terms.
      </p>
    ),
  },
]

const summary = {
  items: [
    { term: 'Who it is for', text: 'Parents, swimmers, coaches and administrators of clubs that use aquify.' },
    { term: 'Kids and accounts', text: 'Parents hold the household account. A swimmer under 13 gets their own only with a parent’s consent.' },
    { term: 'Your club’s decisions', text: 'Fees, rosters and policies belong to the club. aquify provides the tools.' },
    { term: 'Be respectful of data', text: 'Use only what you are authorized to see, and only for the club’s program.' },
  ],
}

export default function TermsPage() {
  return (
    <LegalDoc
      current="terms"
      title="Terms of service"
      summary={summary}
      sections={sections}
    />
  )
}
