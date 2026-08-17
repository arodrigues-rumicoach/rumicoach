// Bun-compatible req/res adapter that mimics Express API
export function bunReqToExpress(req, url) {
  return {
    method: req.method,
    url: url.pathname,
    originalUrl: url.pathname + url.search,
    headers: Object.fromEntries(req.headers),
    query: Object.fromEntries(url.searchParams),
    params: {},
    body: null,
    userId: null,
    region: null,
    isInitialSetup: false,
    ip: req.headers.get('x-forwarded-for') || '127.0.0.1',
    get(header) {
      const key = header.toLowerCase()
      return this.headers[key] || null
    },
  }
}

export function createExpressRes() {
  let _statusCode = 200
  let _headers = {}
  let _body = null
  let finished = false

  const res = {
    get statusCode() { return _statusCode },
    set statusCode(v) { _statusCode = v },
    status(code) {
      _statusCode = code
      return res
    },
    setHeader(name, value) {
      _headers[name.toLowerCase()] = value
    },
    get headersOut() { return { ..._headers } },
    json(body) {
      if (finished) return res
      finished = true
      _headers['content-type'] = 'application/json; charset=utf-8'
      _body = JSON.stringify(body)
      return res
    },
    send(body) {
      if (finished) return res
      finished = true
      if (typeof body !== 'string') {
        _body = JSON.stringify(body)
        _headers['content-type'] = 'application/json; charset=utf-8'
      } else {
        _body = body
      }
      return res
    },
    end(body) {
      if (finished) return res
      finished = true
      if (body !== undefined) _body = body
      return res
    },
    get finished() { return finished },
    get _body() { return _body },
    get resolvedResponse() {
      const headers = new Headers()
      for (const [name, value] of Object.entries(_headers)) {
        headers.set(name, value)
      }
      return new Response(_body, { status: _statusCode, headers })
    },
  }
  return res
}
