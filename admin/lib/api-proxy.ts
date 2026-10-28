// Resolve the internal upstream on each request so one standalone image can be
// promoted between environments without baking a container address into routes.
const hopByHopHeaders = [
  'connection', 'keep-alive', 'proxy-authenticate', 'proxy-authorization',
  'te', 'trailer', 'transfer-encoding', 'upgrade',
]

function proxyHeaders(source: Headers): Headers {
  const headers = new Headers(source)
  const connectionHeaders = headers.get('connection')?.split(',') ?? []
  for (const name of [...hopByHopHeaders, ...connectionHeaders]) {
    if (name.trim()) headers.delete(name.trim())
  }
  return headers
}

export async function proxyAPIRequest(request: Request): Promise<Response> {
  const origin = process.env.API_INTERNAL_URL ?? process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'
  const incoming = new URL(request.url)
  const target = new URL(origin)
  target.pathname = incoming.pathname
  target.search = incoming.search
  const headers = proxyHeaders(request.headers)
  headers.delete('host')
  headers.delete('content-length')
  const hasBody = request.method !== 'GET' && request.method !== 'HEAD'
  const options: RequestInit & { duplex?: 'half' } = {
    method: request.method,
    headers,
    body: hasBody ? request.body : undefined,
    ...(hasBody ? { duplex: 'half' as const } : {}),
    redirect: 'manual',
    cache: 'no-store',
    signal: request.signal,
  }
  try {
    const upstream = await fetch(target, options)
    const responseHeaders = proxyHeaders(upstream.headers)
    // fetch decodes compressed responses; do not forward the original encoding
    // or byte count for the decoded stream.
    responseHeaders.delete('content-encoding')
    responseHeaders.delete('content-length')
    responseHeaders.delete('set-cookie')
    for (const cookie of upstream.headers.getSetCookie()) responseHeaders.append('set-cookie', cookie)
    return new Response(upstream.body, { status: upstream.status, statusText: upstream.statusText, headers: responseHeaders })
  } catch (error) {
    if (request.signal.aborted) throw error
    return Response.json({ error: { code: 'upstream_unavailable', message: 'API is temporarily unavailable.' } }, { status: 502 })
  }
}
