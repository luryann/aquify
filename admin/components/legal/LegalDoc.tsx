import Link from 'next/link'
import type { ReactNode } from 'react'
import { LEGAL_CONTACT_EMAIL, LEGAL_EFFECTIVE_DATE, PRIVACY_PATH, TERMS_PATH } from '@/lib/legal'

export type LegalSection = { id: string; title: string; body: ReactNode }

type Props = {
  title: string
  /** Short plain-language summary shown before the full text. */
  summary?: { items: { term: string; text: ReactNode }[] }
  sections: LegalSection[]
  current: 'privacy' | 'terms'
}

/** Shared frame for the privacy policy and terms: contents rail + reading column. */
export function LegalDoc({ title, summary, sections, current }: Props) {
  return (
    <div className="cs lg">
      <a className="lg-skip" href="#doc">Skip to the document</a>
      <header className="cs-header">
        <div className="cs-wrap cs-header-in">
          <Link href="/" className="cs-brand">aquify</Link>
          <nav className="cs-nav" aria-label="Legal documents">
            <Link href={PRIVACY_PATH} aria-current={current === 'privacy' ? 'page' : undefined}>Privacy policy</Link>
            <Link href={TERMS_PATH} aria-current={current === 'terms' ? 'page' : undefined}>Terms of service</Link>
          </nav>
        </div>
      </header>

      <main id="main" className="cs-wrap lg-main">
        <div className="lg-head">
          <h1>{title}</h1>
          <p className="lg-meta">Effective {LEGAL_EFFECTIVE_DATE}</p>
        </div>

        <div className="lg-col">
          <nav className="lg-toc" aria-label="Sections">
            <ul>
              {sections.map((s) => (
                <li key={s.id}><a href={`#${s.id}`}>{s.title}</a></li>
              ))}
            </ul>
          </nav>

          <article id="doc" className="lg-doc">
            {summary ? (
              <aside className="lg-summary" aria-label="Summary">
                <dl>
                  {summary.items.map((item) => (
                    <div key={item.term}>
                      <dt>{item.term}</dt>
                      <dd>{item.text}</dd>
                    </div>
                  ))}
                </dl>
              </aside>
            ) : null}

            {sections.map((s) => (
              <section key={s.id} id={s.id} className="lg-sec">
                <h2>{s.title}</h2>
                {s.body}
              </section>
            ))}

            <p className="lg-contact">
              Questions about this document? Email <a href={`mailto:${LEGAL_CONTACT_EMAIL}`}>{LEGAL_CONTACT_EMAIL}</a>.
            </p>
          </article>
        </div>
      </main>

      <footer className="cs-footer">
        <div className="cs-wrap cs-footer-in">
          <span>aquify</span>
          <span className="lg-foot-links">
            <Link href={PRIVACY_PATH}>Privacy policy</Link>
            <Link href={TERMS_PATH}>Terms of service</Link>
            <Link href="/login">Sign in</Link>
          </span>
        </div>
      </footer>
    </div>
  )
}
