import type { TestInfo } from '@playwright/test'
import { createHash } from 'node:crypto'

function slugify(value: string) {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export function scenarioFixture(testInfo: TestInfo) {
  const slug = slugify([
    ...testInfo.titlePath.slice(1),
    `repeat-${testInfo.repeatEachIndex}`,
    `retry-${testInfo.retry}`,
  ].join('-'))
  const uuidFor = (value: string) => {
    const hex = createHash('sha256').update(value).digest('hex')
    return [
      hex.slice(0, 8),
      hex.slice(8, 12),
      `4${hex.slice(13, 16)}`,
      `${((Number.parseInt(hex.slice(16, 18), 16) & 0x3f) | 0x80).toString(16).padStart(2, '0')}${hex.slice(18, 20)}`,
      hex.slice(20, 32),
    ].join('-')
  }

  return {
    orgId: uuidFor(`${slug}:org`),
    userId(label: string) {
      return uuidFor(`${slug}:${label}`)
    },
    email(label: string) {
      return `${slugify(label)}-${slug}@example.test`
    },
    text(label: string) {
      return `${label} ${slug}`
    },
    code(label: string, length = 12) {
      return slugify(`${label}-${slug}`).replace(/-/g, '').slice(0, length).toUpperCase()
    },
  }
}
