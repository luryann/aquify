import type { MetadataRoute } from 'next'
import { BRAND_BG } from '@/lib/brand'

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: 'Aquify',
    short_name: 'Aquify',
    description: 'Swim club schedules, meets, and billing.',
    id: '/',
    // /login forwards signed-in users to the right home for their role.
    start_url: '/login',
    scope: '/',
    display: 'standalone',
    orientation: 'portrait',
    background_color: BRAND_BG,
    theme_color: BRAND_BG,
    icons: [
      { src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
      { src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'any' },
      { src: '/icons/icon-maskable-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
    ],
  }
}
