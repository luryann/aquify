import { proxyAPIRequest } from '@/lib/api-proxy'

export const runtime = 'nodejs'
export const dynamic = 'force-dynamic'
export const GET = proxyAPIRequest
export const HEAD = proxyAPIRequest
export const POST = proxyAPIRequest
export const PUT = proxyAPIRequest
export const PATCH = proxyAPIRequest
export const DELETE = proxyAPIRequest
export const OPTIONS = proxyAPIRequest
