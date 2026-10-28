// Generates PWA PNG icons from the brand mark. Run: node scripts/generate-pwa-icons.mjs
import sharp from 'sharp'
import { mkdirSync } from 'node:fs'

const BG = '#0A0A0B'
// Mark geometry from the landing page logo (34x34 viewBox).
const mark = (scale, offset) => `
  <g transform="translate(${offset} ${offset}) scale(${scale})">
    <path d="M9 25V9h7c4.4 0 8 3.58 8 8s-3.6 8-8 8H9z" fill="#eb5d1e"/>
    <path d="M13 21v-8h3c2.2 0 4 1.79 4 4s-1.8 4-4 4h-3z" fill="${BG}"/>
  </g>`
// `inset` = fraction of canvas used by the 34-unit box (maskable needs a safe zone).
const svg = (size, inset, radius) => {
  const scale = (size * inset) / 34
  const offset = (size - size * inset) / 2
  return Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 ${size} ${size}">
  <rect width="${size}" height="${size}" rx="${radius}" fill="${BG}"/>${mark(scale, offset)}</svg>`)
}
mkdirSync('public/icons', { recursive: true })
const jobs = [
  ['icon-192.png', 192, 0.62, 0],
  ['icon-512.png', 512, 0.62, 0],
  ['icon-maskable-512.png', 512, 0.5, 0],
  ['apple-touch-icon.png', 180, 0.62, 0],
]
for (const [name, size, inset, r] of jobs) {
  await sharp(svg(size, inset, r)).png().toFile(`public/icons/${name}`)
}
console.log('icons written')
