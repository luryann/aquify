import type { Metadata, Viewport } from 'next'
import './globals.css'
import { BRAND_BG } from '@/lib/brand'
import { Providers } from './providers'
import { ServiceWorkerRegistrar } from '@/components/pwa/ServiceWorkerRegistrar'

export const metadata: Metadata = {
  title: {
    default: 'Aquify',
    template: '%s · Aquify',
  },
  description: 'Aquify — swim club operations platform.',
  applicationName: 'Aquify',
  appleWebApp: { capable: true, title: 'Aquify', statusBarStyle: 'black-translucent' },
  icons: { apple: '/icons/apple-touch-icon.png' },
  other: { 'apple-mobile-web-app-capable': 'yes' },
}

export const viewport: Viewport = {
  viewportFit: 'cover',
  // The app renders dark by default regardless of the OS preference, so the
  // browser chrome colour is a single value shared with the PWA manifest.
  themeColor: BRAND_BG,
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" data-theme="dark">
      <body>
        <Providers>{children}</Providers>
        <ServiceWorkerRegistrar />
      </body>
    </html>
  )
}
